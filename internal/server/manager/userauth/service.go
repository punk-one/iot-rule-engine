package userauth

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultAdminUsername        = "admin"
	initialAdminPasswordEnv     = "IOT_RULE_ADMIN_PASSWORD"
	sessionTTL                  = 24 * time.Hour
	sessionCleanupInterval      = time.Minute
	maxSessionsPerUser          = 10
	maxSessionsTotal            = 10000
	maxConcurrentPasswordChecks = 4
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserDisabled       = errors.New("user is disabled")
	ErrInvalidSession     = errors.New("invalid session")
	ErrUserExists         = errors.New("user already exists")
	ErrPasswordMismatch   = errors.New("old password is incorrect")
	ErrAuthenticationBusy = errors.New("authentication service is busy")
)

// SysUser 系统用户模型
type SysUser struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Username    string     `gorm:"size:64;uniqueIndex;not null" json:"username"`
	Password    string     `gorm:"size:255;not null" json:"-"`
	Phone       string     `gorm:"size:20;uniqueIndex" json:"phone"`
	Email       string     `gorm:"size:128" json:"email"`
	Status      int        `gorm:"default:1;comment:状态 1-启用 0-禁用" json:"status"`
	AuthVersion int64      `gorm:"default:0;not null" json:"-"`
	Remark      string     `gorm:"size:255" json:"remark"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   *time.Time `gorm:"index" json:"-"`
}

func (SysUser) TableName() string {
	return "sys_users"
}

// PublicUser 返回给前端的用户信息
type PublicUser struct {
	ID          uint       `json:"id"`
	Username    string     `json:"username"`
	Phone       string     `json:"phone"`
	Email       string     `json:"email"`
	Status      int        `json:"status"`
	Remark      string     `json:"remark"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Session 登录会话
type Session struct {
	Token       string    `json:"token"`
	UserID      uint      `json:"user_id"`
	Username    string    `json:"username"`
	ExpiresAt   time.Time `json:"expires_at"`
	authVersion int64
	createdAt   time.Time
}

// Service 用户认证服务
type Service struct {
	db          *sql.DB
	driver      string
	enabled     bool
	mu          sync.RWMutex
	sessions    map[string]Session
	authSlots   chan struct{}
	sessionStop chan struct{}
	sessionDone chan struct{}
	closeOnce   sync.Once
	closeErr    error
}

// CreateUserInput 创建用户入参
type CreateUserInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Status   int    `json:"status"`
	Remark   string `json:"remark"`
}

// Open 初始化认证服务并确保默认管理员存在
func Open(cfg *config.Config) (*Service, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Manager.StorageType))
	if driver == "" {
		driver = "sqlite"
	}

	var (
		db  *sql.DB
		err error
	)

	switch driver {
	case "postgres":
		db, err = openPostgres(cfg)
	default:
		driver = "sqlite"
		db, err = openSQLite(cfg)
	}
	if err != nil {
		return nil, err
	}

	svc := &Service{
		db:          db,
		driver:      driver,
		sessions:    make(map[string]Session),
		authSlots:   make(chan struct{}, maxConcurrentPasswordChecks),
		sessionStop: make(chan struct{}),
		sessionDone: make(chan struct{}),
	}

	if err := svc.initTables(); err != nil {
		db.Close()
		return nil, err
	}
	if err := svc.ensureDefaultAdmin(); err != nil {
		db.Close()
		return nil, err
	}

	svc.enabled = true
	go svc.sessionCleanupLoop()
	return svc, nil
}

// Close 关闭认证服务
func (s *Service) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		close(s.sessionStop)
		<-s.sessionDone
		s.closeErr = s.db.Close()
	})
	return s.closeErr
}

// Enabled 是否启用用户认证
func (s *Service) Enabled() bool {
	return s != nil && s.enabled
}

// Authenticate 校验用户名密码并创建会话
func (s *Service) Authenticate(username, password string) (*PublicUser, *Session, error) {
	user, err := s.findByUsername(strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}

	if user.Status != 1 {
		return nil, nil, ErrUserDisabled
	}

	select {
	case s.authSlots <- struct{}{}:
	default:
		return nil, nil, ErrAuthenticationBusy
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	<-s.authSlots
	if passwordErr != nil {
		return nil, nil, ErrInvalidCredentials
	}

	now := time.Now()
	if err := s.updateLastLogin(user.ID, now); err != nil {
		pkglog.Warn("failed to update last login for user %s: %v", user.Username, err)
	}
	user.LastLoginAt = &now
	user.UpdatedAt = now

	session, err := s.createSession(user)
	if err != nil {
		return nil, nil, err
	}

	return toPublicUser(user), session, nil
}

// ValidateToken 校验登录态
func (s *Service) ValidateToken(token string) (*Session, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrInvalidSession
	}

	s.mu.RLock()
	session, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrInvalidSession
	}

	if time.Now().After(session.ExpiresAt) {
		s.mu.Lock()
		delete(s.sessions, token)
		s.mu.Unlock()
		return nil, ErrInvalidSession
	}

	// Sessions are memory-resident, but authorization state is authoritative in
	// the database. The monotonically increasing auth_version closes races where
	// a login started just before a password reset or account disable completed.
	user, err := s.findByID(session.UserID)
	if err != nil {
		s.deleteSession(token)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	if user.Status != 1 || user.AuthVersion != session.authVersion {
		s.deleteSession(token)
		return nil, ErrInvalidSession
	}

	return &session, nil
}

// Logout 注销登录态
func (s *Service) Logout(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}

	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// GetPublicUser 根据 ID 获取公开用户信息
func (s *Service) GetPublicUser(userID uint) (*PublicUser, error) {
	user, err := s.findByID(userID)
	if err != nil {
		return nil, err
	}
	return toPublicUser(user), nil
}

// ListUsers 列出所有有效用户
func (s *Service) ListUsers() ([]*PublicUser, error) {
	query := `SELECT id, username, password, phone, email, status, auth_version, remark, last_login_at, created_at, updated_at, deleted_at
		FROM sys_users WHERE deleted_at IS NULL ORDER BY created_at ASC, id ASC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	users := make([]*PublicUser, 0)
	for rows.Next() {
		user, err := s.scanUserFromRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, toPublicUser(user))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate users: %w", err)
	}

	return users, nil
}

// CreateUser 创建系统用户
func (s *Service) CreateUser(input CreateUserInput) (*PublicUser, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Email = strings.TrimSpace(input.Email)
	input.Remark = strings.TrimSpace(input.Remark)

	if input.Username == "" || input.Password == "" {
		return nil, fmt.Errorf("username and password are required")
	}
	if input.Status != 0 {
		input.Status = 1
	}

	_, err := s.findByUsername(input.Username)
	if err == nil {
		return nil, ErrUserExists
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	query := fmt.Sprintf(
		`INSERT INTO sys_users (username, password, phone, email, status, remark, created_at, updated_at) VALUES (%s, %s, %s, %s, %s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.placeholder(6), s.placeholder(7), s.placeholder(8),
	)

	if _, err := s.db.Exec(query, input.Username, passwordHash, nullString(input.Phone), nullString(input.Email), input.Status, nullString(input.Remark), now, now); err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	user, err := s.findByUsername(input.Username)
	if err != nil {
		return nil, err
	}
	return toPublicUser(user), nil
}

// ChangePassword 修改当前用户密码
func (s *Service) ChangePassword(userID uint, oldPassword, newPassword string) error {
	user, err := s.findByID(userID)
	if err != nil {
		return err
	}

	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)) != nil {
		return ErrPasswordMismatch
	}

	return s.updatePassword(user.ID, newPassword)
}

// SetPassword 管理员直接重置密码
func (s *Service) SetPassword(userID uint, newPassword string) error {
	_, err := s.findByID(userID)
	if err != nil {
		return err
	}
	return s.updatePassword(userID, newPassword)
}

// UpdateUserStatus 更新用户启用状态
func (s *Service) UpdateUserStatus(userID uint, status int) error {
	if status != 0 && status != 1 {
		return fmt.Errorf("status must be 0 or 1")
	}
	query := fmt.Sprintf(
		`UPDATE sys_users SET status = %s, auth_version = auth_version + 1, updated_at = %s WHERE id = %s AND deleted_at IS NULL`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3),
	)

	result, err := s.db.Exec(query, status, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("failed to update user status: %w", err)
	}

	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	s.revokeUserSessions(userID)

	return nil
}

func openSQLite(cfg *config.Config) (*sql.DB, error) {
	dbDir := strings.TrimSpace(cfg.Manager.DBPath)
	if dbDir == "" {
		dbDir = "data"
	}
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create auth db directory: %w", err)
	}

	dbPath := filepath.Join(dbDir, "rules.db")
	dsn := fmt.Sprintf("%s?_journal=WAL&_busy_timeout=5000&_foreign_keys=on&_loc=auto", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite auth database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping sqlite auth database: %w", err)
	}
	return db, nil
}

func openPostgres(cfg *config.Config) (*sql.DB, error) {
	sslMode := strings.TrimSpace(cfg.Database.SSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		sslMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres auth database: %w", err)
	}

	if cfg.Database.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	}
	if cfg.Database.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	}
	if cfg.Database.ConnMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetimeMinutes) * time.Minute)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping postgres auth database: %w", err)
	}
	return db, nil
}

func (s *Service) initTables() error {
	queries := sqliteQueries
	if s.driver == "postgres" {
		queries = postgresQueries
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to initialize auth tables: %w", err)
		}
	}
	return s.ensureAuthVersionColumn()
}

func (s *Service) ensureAuthVersionColumn() error {
	if s.driver == "postgres" {
		_, err := s.db.Exec(`ALTER TABLE sys_users ADD COLUMN IF NOT EXISTS auth_version BIGINT NOT NULL DEFAULT 0`)
		return err
	}

	rows, err := s.db.Query(`PRAGMA table_info(sys_users)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		if name == "auth_version" {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE sys_users ADD COLUMN auth_version INTEGER NOT NULL DEFAULT 0`)
	return err
}

func (s *Service) ensureDefaultAdmin() error {
	_, err := s.findByUsername(defaultAdminUsername)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	initialPassword := strings.TrimSpace(os.Getenv(initialAdminPasswordEnv))
	if initialPassword == "" {
		return fmt.Errorf("initial admin password is required in %s", initialAdminPasswordEnv)
	}
	passwordHash, err := hashPassword(initialPassword)
	if err != nil {
		return fmt.Errorf("invalid initial admin password from %s: %w", initialAdminPasswordEnv, err)
	}

	now := time.Now()
	query := fmt.Sprintf(
		`INSERT INTO sys_users (username, password, status, remark, created_at, updated_at) VALUES (%s, %s, %s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.placeholder(6),
	)

	if _, err := s.db.Exec(query, defaultAdminUsername, string(passwordHash), 1, "system initialized admin user", now, now); err != nil {
		return fmt.Errorf("failed to seed default admin user: %w", err)
	}

	pkglog.Info("initialized default manager admin user: %s", defaultAdminUsername)
	return nil
}

func (s *Service) findByUsername(username string) (*SysUser, error) {
	query := fmt.Sprintf(
		`SELECT id, username, password, phone, email, status, auth_version, remark, last_login_at, created_at, updated_at, deleted_at
		FROM sys_users WHERE username = %s AND deleted_at IS NULL LIMIT 1`,
		s.placeholder(1),
	)

	return s.scanUser(s.db.QueryRow(query, username))
}

func (s *Service) findByID(userID uint) (*SysUser, error) {
	query := fmt.Sprintf(
		`SELECT id, username, password, phone, email, status, auth_version, remark, last_login_at, created_at, updated_at, deleted_at
		FROM sys_users WHERE id = %s AND deleted_at IS NULL LIMIT 1`,
		s.placeholder(1),
	)

	return s.scanUser(s.db.QueryRow(query, userID))
}

func (s *Service) scanUser(row *sql.Row) (*SysUser, error) {
	var (
		user        SysUser
		phone       sql.NullString
		email       sql.NullString
		remark      sql.NullString
		lastLoginAt sql.NullTime
		deletedAt   sql.NullTime
	)
	if err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&phone,
		&email,
		&user.Status,
		&user.AuthVersion,
		&remark,
		&lastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&deletedAt,
	); err != nil {
		return nil, err
	}

	if phone.Valid {
		user.Phone = phone.String
	}
	if email.Valid {
		user.Email = email.String
	}
	if remark.Valid {
		user.Remark = remark.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}
	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.Time
	}
	return &user, nil
}

func (s *Service) scanUserFromRows(rows *sql.Rows) (*SysUser, error) {
	var (
		user        SysUser
		phone       sql.NullString
		email       sql.NullString
		remark      sql.NullString
		lastLoginAt sql.NullTime
		deletedAt   sql.NullTime
	)
	if err := rows.Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&phone,
		&email,
		&user.Status,
		&user.AuthVersion,
		&remark,
		&lastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&deletedAt,
	); err != nil {
		return nil, err
	}

	if phone.Valid {
		user.Phone = phone.String
	}
	if email.Valid {
		user.Email = email.String
	}
	if remark.Valid {
		user.Remark = remark.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}
	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.Time
	}

	return &user, nil
}

func (s *Service) updateLastLogin(userID uint, loginAt time.Time) error {
	query := fmt.Sprintf(
		`UPDATE sys_users SET last_login_at = %s, updated_at = %s WHERE id = %s`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3),
	)
	_, err := s.db.Exec(query, loginAt, loginAt, userID)
	return err
}

func (s *Service) updatePassword(userID uint, password string) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("password is required")
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(
		`UPDATE sys_users SET password = %s, auth_version = auth_version + 1, updated_at = %s WHERE id = %s AND deleted_at IS NULL`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3),
	)
	result, err := s.db.Exec(query, passwordHash, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	s.revokeUserSessions(userID)

	return nil
}

func (s *Service) createSession(user *SysUser) (*Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate session token: %w", err)
	}

	session := Session{
		Token:       base64.RawURLEncoding.EncodeToString(tokenBytes),
		UserID:      user.ID,
		Username:    user.Username,
		ExpiresAt:   time.Now().Add(sessionTTL),
		authVersion: user.AuthVersion,
		createdAt:   time.Now(),
	}

	s.mu.Lock()
	s.cleanupExpiredSessionsLocked(time.Now())
	s.enforceSessionLimitsLocked(user.ID)
	s.sessions[session.Token] = session
	s.mu.Unlock()

	return &session, nil
}

func (s *Service) enforceSessionLimitsLocked(userID uint) {
	for {
		count := 0
		oldestToken := ""
		var oldest time.Time
		for token, session := range s.sessions {
			if session.UserID != userID {
				continue
			}
			count++
			if oldestToken == "" || session.createdAt.Before(oldest) {
				oldestToken, oldest = token, session.createdAt
			}
		}
		if count < maxSessionsPerUser {
			break
		}
		delete(s.sessions, oldestToken)
	}
	for len(s.sessions) >= maxSessionsTotal {
		oldestToken := ""
		var oldest time.Time
		for token, session := range s.sessions {
			if oldestToken == "" || session.createdAt.Before(oldest) {
				oldestToken, oldest = token, session.createdAt
			}
		}
		if oldestToken == "" {
			break
		}
		delete(s.sessions, oldestToken)
	}
}

func (s *Service) sessionCleanupLoop() {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()
	defer close(s.sessionDone)
	for {
		select {
		case now := <-ticker.C:
			s.mu.Lock()
			s.cleanupExpiredSessionsLocked(now)
			s.mu.Unlock()
		case <-s.sessionStop:
			return
		}
	}
}

func (s *Service) cleanupExpiredSessionsLocked(now time.Time) {
	for token, session := range s.sessions {
		if !session.ExpiresAt.After(now) {
			delete(s.sessions, token)
		}
	}
}

func (s *Service) deleteSession(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *Service) revokeUserSessions(userID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, session := range s.sessions {
		if session.UserID == userID {
			delete(s.sessions, token)
		}
	}
}

func (s *Service) placeholder(index int) string {
	if s.driver == "postgres" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

func toPublicUser(user *SysUser) *PublicUser {
	return &PublicUser{
		ID:          user.ID,
		Username:    user.Username,
		Phone:       user.Phone,
		Email:       user.Email,
		Status:      user.Status,
		Remark:      user.Remark,
		LastLoginAt: user.LastLoginAt,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}

func hashPassword(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(passwordHash), nil
}

func validatePassword(password string) error {
	if len(password) < 12 {
		return fmt.Errorf("password must contain at least 12 characters")
	}
	if len(password) > 72 {
		return fmt.Errorf("password must not exceed 72 bytes")
	}
	var upper, lower, digit, symbol bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		default:
			symbol = true
		}
	}
	if !upper || !lower || !digit || !symbol {
		return fmt.Errorf("password must include upper-case, lower-case, digit and symbol characters")
	}
	return nil
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	lowerErr := strings.ToLower(err.Error())
	return strings.Contains(lowerErr, "unique constraint") ||
		strings.Contains(lowerErr, "duplicate key") ||
		strings.Contains(lowerErr, "unique failed")
}

var sqliteQueries = []string{
	`CREATE TABLE IF NOT EXISTS sys_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL,
		phone TEXT UNIQUE,
		email TEXT,
		status INTEGER DEFAULT 1,
		auth_version INTEGER NOT NULL DEFAULT 0,
		remark TEXT,
		last_login_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		deleted_at DATETIME
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sys_users_deleted_at ON sys_users(deleted_at)`,
}

var postgresQueries = []string{
	`CREATE TABLE IF NOT EXISTS sys_users (
		id SERIAL PRIMARY KEY,
		username VARCHAR(64) UNIQUE NOT NULL,
		password VARCHAR(255) NOT NULL,
		phone VARCHAR(20) UNIQUE,
		email VARCHAR(128),
		status INTEGER DEFAULT 1,
		auth_version BIGINT NOT NULL DEFAULT 0,
		remark VARCHAR(255),
		last_login_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		deleted_at TIMESTAMPTZ
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sys_users_deleted_at ON sys_users(deleted_at)`,
}
