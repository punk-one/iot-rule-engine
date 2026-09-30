package handlers

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

const fallbackConfigPath = "configs/config.yaml"
const maxConfigCertUploadBytes int64 = 2 << 20

// GetConfigHandler 获取当前配置（优先从数据库，否则从默认 YAML）
func (m *Manager) GetConfigHandler(c *gin.Context) {
	var cfg *config.Config
	var err error

	// 1. 优先从数据库读取配置
	configData, dbErr := m.db.GetConfig()
	if dbErr == nil && len(configData) > 0 {
		// 成功从数据库读取配置
		cfg = &config.Config{}
		if err = yaml.Unmarshal(configData, cfg); err == nil {
			pkglog.Info("Loaded configuration from database")
			c.JSON(http.StatusOK, gin.H{
				"config": cfg,
				"source": "database",
			})
			return
		}
		pkglog.Warn("Failed to unmarshal config from database: %v", err)
	}

	// 2. 从默认 YAML 文件读取
	data, err := os.ReadFile(fallbackConfigPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to read default config file: %v", err),
		})
		return
	}

	cfg = &config.Config{}
	if err = yaml.Unmarshal(data, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to unmarshal config: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"config": cfg,
		"source": "default_yaml",
	})
}

// UpdateConfigHandler 更新配置内容（保存到数据库）
func (m *Manager) UpdateConfigHandler(c *gin.Context) {
	var req struct {
		Config *config.Config `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.Config == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "config cannot be empty"})
		return
	}

	if current, err := m.loadCurrentConfig(); err == nil {
		preserveHiddenConfig(current, req.Config)
		if current.Manager.Port != req.Config.Manager.Port ||
			current.Manager.DBPath != req.Config.Manager.DBPath ||
			current.Manager.StorageType != req.Config.Manager.StorageType ||
			current.Manager.InitDB != req.Config.Manager.InitDB {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "manager port and storage location are bootstrap settings; change the startup YAML or environment and restart",
			})
			return
		}
	}
	if err := config.NormalizeAndValidate(req.Config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 将配置序列化为 YAML
	configData, err := yaml.Marshal(req.Config)
	if err != nil {
		pkglog.Error("Failed to marshal config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal config"})
		return
	}

	// 保存到数据库
	if err := m.db.SaveConfig(configData); err != nil {
		pkglog.Error("Failed to save config to database: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config to database"})
		return
	}

	pkglog.Info("Configuration saved to database successfully")
	c.JSON(http.StatusOK, gin.H{
		"message": "configuration saved successfully. Please restart the service to apply changes.",
		"source":  "database",
	})
}

// UploadConfigCertHandler 上传证书文件
func (m *Manager) UploadConfigCertHandler(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxConfigCertUploadBytes)
	file, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds 2 MiB limit"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	if file.Size > maxConfigCertUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds 2 MiB limit"})
		return
	}
	upload, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open uploaded file"})
		return
	}
	data, readErr := io.ReadAll(io.LimitReader(upload, maxConfigCertUploadBytes+1))
	_ = upload.Close()
	if readErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read uploaded file"})
		return
	}
	if int64(len(data)) > maxConfigCertUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds 2 MiB limit"})
		return
	}
	if err := validateCertificatePEM(data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	certDir := "data/certs"
	if err := os.MkdirAll(certDir, 0700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create certs directory"})
		return
	}
	_ = os.Chmod(certDir, 0700)

	nameBytes := make([]byte, 16)
	if _, err := rand.Read(nameBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to allocate file name"})
		return
	}
	filePath := filepath.Join(certDir, hex.EncodeToString(nameBytes)+".pem")
	destination, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create certificate file"})
		return
	}
	if _, err := destination.Write(data); err != nil {
		_ = destination.Close()
		_ = os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save certificate file"})
		return
	}
	if err := destination.Close(); err != nil {
		_ = os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize certificate file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":    filePath,
		"message": "file uploaded successfully",
	})
}

func (m *Manager) loadCurrentConfig() (*config.Config, error) {
	data, err := m.db.GetConfig()
	if err != nil || len(data) == 0 {
		data, err = os.ReadFile(fallbackConfigPath)
	}
	if err != nil {
		return nil, err
	}
	cfg := &config.Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func preserveHiddenConfig(current, incoming *config.Config) {
	if current == nil || incoming == nil {
		return
	}
	incoming.Database.Password = current.Database.Password
	incoming.Redis.Password = current.Redis.Password
	incoming.Common.NATS.Password = current.Common.NATS.Password
	incoming.Common.NATS.Token = current.Common.NATS.Token
	incoming.Security.Manager.Token = current.Security.Manager.Token
}

func validateCertificatePEM(data []byte) error {
	rest := data
	blocks := 0
	for len(strings.TrimSpace(string(rest))) > 0 {
		block, remaining := pem.Decode(rest)
		if block == nil {
			return fmt.Errorf("file must contain only valid PEM certificate or key blocks")
		}
		switch block.Type {
		case "CERTIFICATE":
			if _, err := x509.ParseCertificate(block.Bytes); err != nil {
				return fmt.Errorf("invalid X.509 certificate")
			}
		case "PRIVATE KEY":
			if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
				return fmt.Errorf("invalid PKCS#8 private key")
			}
		case "RSA PRIVATE KEY":
			if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
				return fmt.Errorf("invalid RSA private key")
			}
		case "EC PRIVATE KEY":
			if _, err := x509.ParseECPrivateKey(block.Bytes); err != nil {
				return fmt.Errorf("invalid EC private key")
			}
		case "PUBLIC KEY":
			key, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return fmt.Errorf("invalid public key")
			}
			switch key.(type) {
			case *rsa.PublicKey, *ecdsa.PublicKey:
			default:
				return fmt.Errorf("unsupported public key type")
			}
		default:
			return fmt.Errorf("unsupported PEM block type %q", block.Type)
		}
		blocks++
		rest = remaining
	}
	if blocks == 0 {
		return fmt.Errorf("file does not contain a PEM certificate or key")
	}
	return nil
}
