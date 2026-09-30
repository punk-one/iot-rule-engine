package errors

import (
	"errors"
	"net/http"
	"testing"
)

func TestNewRuleNotFoundError(t *testing.T) {
	err := NewRuleNotFoundError("rule-123")

	if err.Code != ErrCodeRuleNotFound {
		t.Errorf("Expected code %s, got %s", ErrCodeRuleNotFound, err.Code)
	}

	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("Expected HTTP status %d, got %d", http.StatusNotFound, err.HTTPStatus)
	}

	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

func TestNewRuleAlreadyExistsError(t *testing.T) {
	err := NewRuleAlreadyExistsError("rule-123")

	if err.Code != ErrCodeRuleAlreadyExists {
		t.Errorf("Expected code %s, got %s", ErrCodeRuleAlreadyExists, err.Code)
	}

	if err.HTTPStatus != http.StatusConflict {
		t.Errorf("Expected HTTP status %d, got %d", http.StatusConflict, err.HTTPStatus)
	}
}

func TestNewRuleValidationError(t *testing.T) {
	details := map[string]string{"field": "name", "error": "required"}
	err := NewRuleValidationError(details)

	if err.Code != ErrCodeRuleValidation {
		t.Errorf("Expected code %s, got %s", ErrCodeRuleValidation, err.Code)
	}

	if err.Details == nil {
		t.Error("Details should not be nil")
	}

	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf("Expected HTTP status %d, got %d", http.StatusBadRequest, err.HTTPStatus)
	}
}

func TestAppError_WithCause(t *testing.T) {
	cause := errors.New("original error")
	err := NewInternalError(nil).WithCause(cause)

	if err.Cause != cause {
		t.Error("Cause should be set")
	}

	// Test Unwrap
	unwrapped := errors.Unwrap(err)
	if unwrapped != cause {
		t.Error("Unwrap should return the cause")
	}
}

func TestAppError_WithDetails(t *testing.T) {
	details := map[string]interface{}{
		"key": "value",
	}
	err := NewInternalError(nil).WithDetails(details)

	if err.Details == nil {
		t.Error("Details should not be nil")
	}

	detailsMap, ok := err.Details.(map[string]interface{})
	if !ok {
		t.Error("Details should be a map")
	}

	if detailsMap["key"] != "value" {
		t.Error("Details should contain the correct value")
	}
}

func TestIsAppError(t *testing.T) {
	appErr := NewRuleNotFoundError("test")
	if !IsAppError(appErr) {
		t.Error("Should return true for AppError")
	}

	stdErr := errors.New("standard error")
	if IsAppError(stdErr) {
		t.Error("Should return false for standard error")
	}
}

func TestAsAppError(t *testing.T) {
	// Test with AppError
	appErr := NewRuleNotFoundError("test")
	result := AsAppError(appErr)
	if result.Code != ErrCodeRuleNotFound {
		t.Error("Should return the same AppError")
	}

	// Test with standard error
	stdErr := errors.New("standard error")
	result = AsAppError(stdErr)
	if result.Code != ErrCodeInternalError {
		t.Errorf("Should wrap standard error as internal error, got %s", result.Code)
	}
	if result.Cause != stdErr {
		t.Error("Should set the original error as cause")
	}
}

func TestGetHTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"RuleNotFound", NewRuleNotFoundError("test"), http.StatusNotFound},
		{"RuleAlreadyExists", NewRuleAlreadyExistsError("test"), http.StatusConflict},
		{"Validation", NewRuleValidationError(nil), http.StatusBadRequest},
		{"Internal", NewInternalError(nil), http.StatusInternalServerError},
		{"Unauthorized", NewUnauthorizedError(), http.StatusUnauthorized},
		{"Forbidden", NewForbiddenError(), http.StatusForbidden},
		{"StandardError", errors.New("test"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := GetHTTPStatus(tt.err)
			if status != tt.expected {
				t.Errorf("Expected status %d, got %d", tt.expected, status)
			}
		})
	}
}

func TestAppError_Error(t *testing.T) {
	// Without cause
	err := NewRuleNotFoundError("test")
	errStr := err.Error()
	if errStr == "" {
		t.Error("Error string should not be empty")
	}

	// With cause
	cause := errors.New("original error")
	err = NewInternalError(nil).WithCause(cause)
	errStr = err.Error()
	if errStr == "" {
		t.Error("Error string should not be empty with cause")
	}
}
