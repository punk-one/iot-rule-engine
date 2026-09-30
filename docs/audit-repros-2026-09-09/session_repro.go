//go:build ignore

// Run from the engine repository: go run docs/audit-repros-2026-09-09/session_repro.go
// Uses an isolated temporary database; never touches the running engine.
package main

import (
	"fmt"
	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	"github.com/punk-one/iot-rule-engine/internal/server/manager/userauth"
	"os"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	dir, err := os.MkdirTemp("", "iot-audit-auth-")
	must(err)
	defer os.RemoveAll(dir)
	must(os.Setenv("IOT_RULE_ADMIN_PASSWORD", "AuditOnly@123456"))
	svc, err := userauth.Open(&config.Config{Manager: config.ManagerConfig{DBPath: dir, StorageType: "sqlite"}})
	must(err)
	defer svc.Close()
	user, err := svc.CreateUser(userauth.CreateUserInput{Username: "audit_user", Password: "AuditOnly@234567", Status: 1})
	must(err)
	_, session, err := svc.Authenticate("audit_user", "AuditOnly@234567")
	must(err)
	must(svc.SetPassword(user.ID, "AuditOnly@345678"))
	_, err = svc.ValidateToken(session.Token)
	fmt.Printf("old_token_accepted_after_password_reset=%v\n", err == nil)
	must(svc.UpdateUserStatus(user.ID, 0))
	_, err = svc.ValidateToken(session.Token)
	fmt.Printf("old_token_accepted_after_disable=%v\n", err == nil)
	_, _, err = svc.Authenticate("audit_user", "AuditOnly@345678")
	fmt.Printf("fresh_login_blocked_after_disable=%v\n", err != nil)
}
