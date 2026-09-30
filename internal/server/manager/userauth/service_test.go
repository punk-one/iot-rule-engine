package userauth

import (
	"testing"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	"github.com/stretchr/testify/require"
)

const testAdminPassword = "Admin@Test123"

func openTestService(t *testing.T) *Service {
	t.Helper()
	t.Setenv(initialAdminPasswordEnv, testAdminPassword)
	cfg := &config.Config{Manager: config.ManagerConfig{DBPath: t.TempDir(), StorageType: "sqlite"}}
	svc, err := Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func TestSessionCapacityAndExpiredCleanup(t *testing.T) {
	svc := openTestService(t)
	user, err := svc.findByUsername(defaultAdminUsername)
	require.NoError(t, err)
	for i := 0; i < maxSessionsPerUser+3; i++ {
		_, err := svc.createSession(user)
		require.NoError(t, err)
	}
	svc.mu.RLock()
	require.Len(t, svc.sessions, maxSessionsPerUser)
	svc.mu.RUnlock()

	svc.mu.Lock()
	svc.sessions["expired"] = Session{Token: "expired", UserID: user.ID, ExpiresAt: time.Now().Add(-time.Second), createdAt: time.Now().Add(-time.Hour)}
	svc.cleanupExpiredSessionsLocked(time.Now())
	_, exists := svc.sessions["expired"]
	svc.mu.Unlock()
	require.False(t, exists)
}

func TestDefaultAdminAuthenticationAndSessionLifecycle(t *testing.T) {
	svc := openTestService(t)

	require.True(t, svc.Enabled())

	user, session, err := svc.Authenticate(defaultAdminUsername, testAdminPassword)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, defaultAdminUsername, user.Username)
	require.NotNil(t, user.LastLoginAt)
	require.NotNil(t, session)
	require.NotEmpty(t, session.Token)

	validated, err := svc.ValidateToken(session.Token)
	require.NoError(t, err)
	require.Equal(t, user.ID, validated.UserID)
	require.Equal(t, user.Username, validated.Username)

	svc.Logout(session.Token)
	_, err = svc.ValidateToken(session.Token)
	require.ErrorIs(t, err, ErrInvalidSession)
}

func TestAuthenticateRejectsWrongPassword(t *testing.T) {
	svc := openTestService(t)

	_, _, err := svc.Authenticate(defaultAdminUsername, "wrong-password")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestCreateUserAndChangePassword(t *testing.T) {
	svc := openTestService(t)

	created, err := svc.CreateUser(CreateUserInput{
		Username: "operator",
		Password: "Operator@123",
		Email:    "operator@example.com",
		Status:   1,
		Remark:   "ops user",
	})
	require.NoError(t, err)
	require.Equal(t, "operator", created.Username)

	users, err := svc.ListUsers()
	require.NoError(t, err)
	require.Len(t, users, 2)

	_, oldSession, err := svc.Authenticate("operator", "Operator@123")
	require.NoError(t, err)

	err = svc.ChangePassword(created.ID, "Operator@123", "Operator@456")
	require.NoError(t, err)
	_, err = svc.ValidateToken(oldSession.Token)
	require.ErrorIs(t, err, ErrInvalidSession)

	_, enabledSession, err := svc.Authenticate("operator", "Operator@456")
	require.NoError(t, err)

	err = svc.UpdateUserStatus(created.ID, 0)
	require.NoError(t, err)
	_, err = svc.ValidateToken(enabledSession.Token)
	require.ErrorIs(t, err, ErrInvalidSession)

	_, _, err = svc.Authenticate("operator", "Operator@456")
	require.ErrorIs(t, err, ErrUserDisabled)
}

func TestAdminPasswordResetRevokesExistingSessions(t *testing.T) {
	svc := openTestService(t)
	user, session, err := svc.Authenticate(defaultAdminUsername, testAdminPassword)
	require.NoError(t, err)

	require.NoError(t, svc.SetPassword(user.ID, "Admin@Changed456"))
	_, err = svc.ValidateToken(session.Token)
	require.ErrorIs(t, err, ErrInvalidSession)
	_, _, err = svc.Authenticate(defaultAdminUsername, "Admin@Changed456")
	require.NoError(t, err)
}

func TestExistingDatabaseMigratesAuthVersion(t *testing.T) {
	t.Setenv(initialAdminPasswordEnv, testAdminPassword)
	dir := t.TempDir()
	db, err := openSQLite(&config.Config{Manager: config.ManagerConfig{DBPath: dir}})
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE sys_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL,
		phone TEXT UNIQUE,
		email TEXT,
		status INTEGER DEFAULT 1,
		remark TEXT,
		last_login_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		deleted_at DATETIME
	)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	svc, err := Open(&config.Config{Manager: config.ManagerConfig{DBPath: dir, StorageType: "sqlite"}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	_, _, err = svc.Authenticate(defaultAdminUsername, testAdminPassword)
	require.NoError(t, err)
}

func TestOpenRequiresInitialAdminSecret(t *testing.T) {
	t.Setenv(initialAdminPasswordEnv, "")
	cfg := &config.Config{Manager: config.ManagerConfig{DBPath: t.TempDir(), StorageType: "sqlite"}}
	_, err := Open(cfg)
	require.ErrorContains(t, err, initialAdminPasswordEnv)
}

func TestPasswordPolicyRejectsWeakPassword(t *testing.T) {
	require.Error(t, validatePassword("short"))
	require.Error(t, validatePassword("alllowercase123!"))
	require.NoError(t, validatePassword(testAdminPassword))
}
