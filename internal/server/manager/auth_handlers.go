package manager

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/internal/server/manager/userauth"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Status   int    `json:"status"`
	Remark   string `json:"remark"`
}

type updateUserPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

type updateUserStatusRequest struct {
	Status int `json:"status"`
}

func (s *Server) LoginHandler(c *gin.Context) {
	if s.authService == nil || !s.authService.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth service unavailable"})
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, session, err := s.authService.Authenticate(req.Username, req.Password)
	if err != nil {
		switch err {
		case userauth.ErrInvalidCredentials, userauth.ErrUserDisabled:
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		case userauth.ErrAuthenticationBusy:
			c.Header("Retry-After", "1")
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		default:
			pkglog.Error("Manager API: login failed for user %s: %v", strings.TrimSpace(req.Username), err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      session.Token,
		"expires_at": session.ExpiresAt,
		"user":       user,
	})
}

func (s *Server) LogoutHandler(c *gin.Context) {
	if s.authService != nil {
		s.authService.Logout(extractBearerToken(c))
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (s *Server) CurrentUserHandler(c *gin.Context) {
	if s.authService == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	user, err := s.authService.GetPublicUser(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (s *Server) ChangeOwnPasswordHandler(c *gin.Context) {
	if s.authService == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := s.authService.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, userauth.ErrPasswordMismatch):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, sql.ErrNoRows):
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change password"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}

func (s *Server) ListUsersHandler(c *gin.Context) {
	if !requireAdminUser(c) {
		return
	}

	users, err := s.authService.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (s *Server) CreateUserHandler(c *gin.Context) {
	if !requireAdminUser(c) {
		return
	}

	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := s.authService.CreateUser(userauth.CreateUserInput{
		Username: req.Username,
		Password: req.Password,
		Phone:    req.Phone,
		Email:    req.Email,
		Status:   req.Status,
		Remark:   req.Remark,
	})
	if err != nil {
		switch {
		case errors.Is(err, userauth.ErrUserExists):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"user": user})
}

func (s *Server) UpdateUserPasswordHandler(c *gin.Context) {
	if !requireAdminUser(c) {
		return
	}

	userID, err := parseUintParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req updateUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := s.authService.SetPassword(userID, req.NewPassword); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password reset"})
}

func (s *Server) UpdateUserStatusHandler(c *gin.Context) {
	if !requireAdminUser(c) {
		return
	}

	userID, err := parseUintParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req updateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	status := 0
	if req.Status != 0 {
		status = 1
	}

	if err := s.authService.UpdateUserStatus(userID, status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "status updated"})
}

func requireAdminUser(c *gin.Context) bool {
	username, _ := c.Get("auth_username")
	if strings.TrimSpace(toString(username)) != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin privilege required"})
		c.Abort()
		return false
	}
	return true
}

func parseUintParam(value string) (uint, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(parsed), nil
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
