package handlers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/punk-one/iot-rule-engine/internal/infra/config"
)

func TestPreserveHiddenConfig(t *testing.T) {
	current := &config.Config{
		Database: config.DatabaseConfig{Password: "database-secret"},
		Redis:    config.RedisConfig{Password: "redis-secret"},
		Common:   config.CommonConfig{NATS: config.NATSConfig{Password: "nats-password", Token: "nats-token"}},
		Security: config.SecurityConfig{Manager: config.ManagerSecurityConfig{Token: "manager-token"}},
	}
	incoming := &config.Config{}
	preserveHiddenConfig(current, incoming)
	if incoming.Database.Password != "database-secret" || incoming.Redis.Password != "redis-secret" ||
		incoming.Common.NATS.Password != "nats-password" || incoming.Common.NATS.Token != "nats-token" ||
		incoming.Security.Manager.Token != "manager-token" {
		t.Fatalf("hidden values were not preserved: %+v", incoming)
	}
}

func TestValidateCertificatePEM(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	valid := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := validateCertificatePEM(valid); err != nil {
		t.Fatalf("expected valid key: %v", err)
	}
	if err := validateCertificatePEM([]byte("not a pem file")); err == nil {
		t.Fatal("expected invalid input to be rejected")
	}
}
