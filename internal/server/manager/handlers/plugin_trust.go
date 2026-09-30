package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type trustedPluginArtifact struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Type     string `json:"type"`
	SDK      string `json:"sdk"`
	Artifact string `json:"artifact"`
	SHA256   string `json:"sha256"`
}

type trustedPluginManifest struct {
	SchemaVersion int                     `json:"schema_version"`
	Plugins       []trustedPluginArtifact `json:"plugins"`
}

func (m *Manager) preparePluginUpload(c *gin.Context) bool {
	if !m.installPolicy.Enabled {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "runtime plugin installation is disabled"})
		return false
	}
	if strings.TrimSpace(m.installPolicy.TrustedManifestPath) == "" {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "trusted plugin manifest is not configured"})
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, m.installPolicy.MaxUploadBytes)
	return true
}

func safeUploadName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name {
		return "", fmt.Errorf("invalid plugin filename")
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".so", ".exe", ".dll":
		return name, nil
	default:
		return "", fmt.Errorf("unsupported plugin artifact extension")
	}
}

func (m *Manager) verifyTrustedArtifact(path, uploadName string) (*trustedPluginArtifact, error) {
	manifestFile, err := os.Open(m.installPolicy.TrustedManifestPath)
	if err != nil {
		return nil, fmt.Errorf("open trusted plugin manifest: %w", err)
	}
	defer manifestFile.Close()

	var manifest trustedPluginManifest
	decoder := json.NewDecoder(io.LimitReader(manifestFile, 4<<20))
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode trusted plugin manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported trusted plugin manifest schema %d", manifest.SchemaVersion)
	}

	var record *trustedPluginArtifact
	for i := range manifest.Plugins {
		candidate := &manifest.Plugins[i]
		if filepath.Base(filepath.FromSlash(candidate.Artifact)) == uploadName {
			if record != nil {
				return nil, fmt.Errorf("trusted manifest contains duplicate artifact %q", uploadName)
			}
			record = candidate
		}
	}
	if record == nil {
		return nil, fmt.Errorf("artifact %q is not present in trusted plugin manifest", uploadName)
	}
	if record.ID == "" || record.Version == "" || record.Type == "" || record.SDK == "" {
		return nil, fmt.Errorf("trusted manifest record for %q is incomplete", uploadName)
	}
	if record.SDK != "github.com/punk-one/iot-rule-plugin-sdk@v1.0.0" {
		return nil, fmt.Errorf("artifact %q does not target the supported SDK v1.0.0", uploadName)
	}
	expectedDigest, err := hex.DecodeString(record.SHA256)
	if err != nil || len(expectedDigest) != sha256.Size {
		return nil, fmt.Errorf("trusted manifest has invalid sha256 for %q", uploadName)
	}

	artifactFile, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open uploaded plugin: %w", err)
	}
	defer artifactFile.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, artifactFile); err != nil {
		return nil, fmt.Errorf("hash uploaded plugin: %w", err)
	}
	if subtle.ConstantTimeCompare(digest.Sum(nil), expectedDigest) != 1 {
		return nil, fmt.Errorf("sha256 mismatch for %q", uploadName)
	}

	copy := *record
	return &copy, nil
}
