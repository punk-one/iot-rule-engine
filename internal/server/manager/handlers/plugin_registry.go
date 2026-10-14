package handlers

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	"github.com/punk-one/iot-rule-engine/pkg/utils"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

const supportedRegistrySDK = "github.com/punk-one/iot-rule-plugin-sdk@v1.0.0"

type registryArtifact struct {
	Plugin     string    `json:"plugin"`
	Version    string    `json:"version"`
	Type       string    `json:"type"`
	SDK        string    `json:"sdk"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	Filename   string    `json:"filename"`
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size"`
	Visibility string    `json:"visibility"`
	Release    string    `json:"release"`
	CreatedAt  time.Time `json:"created_at"`
}

type registryCatalogItem struct {
	Plugin      registryArtifact `json:"plugin"`
	DownloadURL string           `json:"download_url"`
	BundleURL   string           `json:"bundle_url"`
	SBOMURL     string           `json:"sbom_url"`
}

type registryCatalog struct {
	SchemaVersion int                   `json:"schema_version"`
	Artifacts     []registryCatalogItem `json:"artifacts"`
}

type registryInstallRequest struct {
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func (m *Manager) ListRegistryPluginsHandler(c *gin.Context) {
	catalog, err := m.fetchRegistryCatalog(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, catalog)
}

func (m *Manager) InstallRegistryPluginHandler(c *gin.Context) {
	if !m.installPolicy.Enabled || strings.TrimSpace(m.installPolicy.RegistryURL) == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "plugin registry installation is disabled"})
		return
	}
	var request registryInstallRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid install request"})
		return
	}
	if request.OS == "" {
		request.OS = runtime.GOOS
	}
	if request.Arch == "" {
		request.Arch = runtime.GOARCH
	}
	catalog, err := m.fetchRegistryCatalog(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	var selected *registryCatalogItem
	for i := range catalog.Artifacts {
		item := &catalog.Artifacts[i]
		if item.Plugin.Plugin == c.Param("name") && item.Plugin.Version == request.Version && item.Plugin.OS == request.OS && item.Plugin.Arch == request.Arch {
			selected = item
			break
		}
	}
	if selected == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "compatible registry artifact not found"})
		return
	}
	if selected.Plugin.SDK != supportedRegistrySDK {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "plugin targets an unsupported SDK"})
		return
	}

	m.installMu.Lock()
	defer m.installMu.Unlock()
	tempDir, err := os.MkdirTemp("", "plugin-registry-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create install staging directory"})
		return
	}
	defer os.RemoveAll(tempDir)
	artifactPath := filepath.Join(tempDir, selected.Plugin.Filename)
	bundlePath := artifactPath + ".sigstore.json"
	if err := m.downloadRegistryFile(c.Request.Context(), selected.DownloadURL, artifactPath, m.installPolicy.MaxUploadBytes); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("download artifact: %v", err)})
		return
	}
	if err := m.downloadRegistryFile(c.Request.Context(), selected.BundleURL, bundlePath, 4<<20); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("download signature bundle: %v", err)})
		return
	}
	if err := verifyFileSHA256(artifactPath, selected.Plugin.SHA256); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if err := m.verifyRegistrySignature(c.Request.Context(), artifactPath, bundlePath); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if err := os.Chmod(artifactPath, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to secure downloaded plugin"})
		return
	}
	descriptor, err := m.pluginLoader.VerifyPlugin(artifactPath)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Sprintf("plugin handshake failed: %v", err)})
		return
	}
	if descriptor.ID != selected.Plugin.Plugin || descriptor.Version != strings.TrimPrefix(selected.Plugin.Version, "v") || string(descriptor.Type) != selected.Plugin.Type {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "plugin handshake identity does not match registry metadata"})
		return
	}
	client, status, err := m.commitRegistryArtifact(artifactPath, selected.Plugin.Filename, descriptor)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, m.toPluginMeta(client))
}

func (m *Manager) fetchRegistryCatalog(ctx context.Context) (*registryCatalog, error) {
	endpoint, err := m.registryURL("/api/v1/plugins")
	if err != nil {
		return nil, err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if m.installPolicy.RegistryToken != "" {
		request.Header.Set("Authorization", "Bearer "+m.installPolicy.RegistryToken)
	}
	response, err := m.registryHTTPClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("registry request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned HTTP %d", response.StatusCode)
	}
	var result registryCatalog
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode registry catalog: %w", err)
	}
	if result.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported registry catalog schema %d", result.SchemaVersion)
	}
	return &result, nil
}

func (m *Manager) downloadRegistryFile(ctx context.Context, rawURL, destination string, limit int64) error {
	endpoint, err := m.registryURL(rawURL)
	if err != nil {
		return err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if m.installPolicy.RegistryToken != "" {
		request.Header.Set("Authorization", "Bearer "+m.installPolicy.RegistryToken)
	}
	response, err := m.registryHTTPClient().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("registry returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return errors.New("registry file exceeds configured size limit")
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, limit+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > limit {
		return errors.New("registry file exceeds configured size limit")
	}
	return nil
}

func (m *Manager) registryURL(reference string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimRight(m.installPolicy.RegistryURL, "/") + "/")
	if err != nil || base.Host == "" {
		return nil, errors.New("invalid plugin registry URL")
	}
	if base.Scheme != "https" && !(base.Scheme == "http" && (base.Hostname() == "127.0.0.1" || base.Hostname() == "localhost")) {
		return nil, errors.New("plugin registry must use HTTPS")
	}
	ref, err := url.Parse(reference)
	if err != nil {
		return nil, errors.New("invalid registry resource URL")
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != base.Scheme || !strings.EqualFold(resolved.Host, base.Host) {
		return nil, errors.New("registry resource URL changed origin")
	}
	return resolved, nil
}

func (m *Manager) registryHTTPClient() *http.Client {
	return &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			return errors.New("cross-origin registry redirect rejected")
		}
		if len(via) >= 3 {
			return errors.New("too many registry redirects")
		}
		return nil
	}}
}

func (m *Manager) verifyRegistrySignature(ctx context.Context, artifactPath, bundlePath string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.installPolicy.CosignPath, "verify-blob", artifactPath, "--bundle", bundlePath, "--certificate-identity-regexp", m.installPolicy.SigstoreIdentityRegexp, "--certificate-oidc-issuer", m.installPolicy.SigstoreIssuer)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sigstore verification failed: %w: %.2048s", err, output)
	}
	return nil
}

func verifyFileSHA256(path, expected string) error {
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil || len(expectedBytes) != sha256.Size {
		return errors.New("registry returned an invalid SHA-256 digest")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(hash.Sum(nil), expectedBytes) != 1 {
		return errors.New("downloaded plugin SHA-256 does not match registry metadata")
	}
	return nil
}

func (m *Manager) commitRegistryArtifact(source, filename string, descriptor *sdk.PluginInfo) (*plugin.PluginClient, int, error) {
	targetDir := filepath.Join(m.pluginsDir, getPluginSubDir(descriptor.Type))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, http.StatusInternalServerError, errors.New("failed to create plugin directory")
	}
	target := filepath.Join(targetDir, filepath.Base(filename))
	existingID, existing, existingErr := m.resolvePluginClient(descriptor.ID)
	if existingErr != nil {
		if _, err := os.Lstat(target); err == nil {
			return nil, http.StatusConflict, errors.New("plugin artifact already exists")
		} else if !os.IsNotExist(err) {
			return nil, http.StatusInternalServerError, err
		}
		if err := stageAndRename(source, target); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		client, err := m.loadManagedPlugin(target, descriptor.Type)
		if err != nil {
			_ = os.Remove(target)
			return nil, http.StatusUnprocessableEntity, fmt.Errorf("plugin load failed: %w", err)
		}
		if err := updateLatestPluginLink(targetDir, descriptor.ID, target); err != nil {
			_ = m.pluginLoader.UnloadPlugin(descriptor.ID)
			_ = os.Remove(target)
			return nil, http.StatusInternalServerError, fmt.Errorf("update latest plugin link: %w", err)
		}
		return client, http.StatusCreated, nil
	}
	if existingID != descriptor.ID || existing.Info.Type != descriptor.Type {
		return nil, http.StatusConflict, errors.New("installed plugin identity or type does not match registry artifact")
	}
	if existing.Info.Version == descriptor.Version {
		return existing, http.StatusOK, nil
	}
	if _, err := os.Lstat(target); err == nil && filepath.Clean(target) != filepath.Clean(existing.Path) {
		return nil, http.StatusConflict, errors.New("target plugin version already exists")
	}
	staged := target + fmt.Sprintf(".staged-%d", time.Now().UnixNano())
	if err := utils.CopyFile(source, staged); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer os.Remove(staged)
	if err := os.Chmod(staged, 0o755); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if err := m.pluginLoader.UnloadPlugin(existingID); err != nil {
		return nil, http.StatusConflict, fmt.Errorf("plugin is still in use: %w", err)
	}
	if err := os.Rename(staged, target); err != nil {
		_, _ = m.loadManagedPlugin(existing.Path, existing.Info.Type)
		return nil, http.StatusInternalServerError, err
	}
	client, err := m.loadManagedPlugin(target, descriptor.Type)
	if err != nil {
		_ = os.Remove(target)
		_, _ = m.loadManagedPlugin(existing.Path, existing.Info.Type)
		return nil, http.StatusUnprocessableEntity, fmt.Errorf("upgrade load failed: %w", err)
	}
	if err := updateLatestPluginLink(targetDir, descriptor.ID, target); err != nil {
		_ = m.pluginLoader.UnloadPlugin(descriptor.ID)
		_ = os.Remove(target)
		_, _ = m.loadManagedPlugin(existing.Path, existing.Info.Type)
		return nil, http.StatusInternalServerError, fmt.Errorf("update latest plugin link: %w", err)
	}
	return client, http.StatusOK, nil
}

func stageAndRename(source, target string) error {
	temp, err := os.CreateTemp(filepath.Dir(target), ".plugin-registry-*")
	if err != nil {
		return err
	}
	path := temp.Name()
	_ = temp.Close()
	defer os.Remove(path)
	if err := utils.CopyFile(source, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return err
	}
	return os.Rename(path, target)
}

func updateLatestPluginLink(directory, pluginID, target string) error {
	latest := filepath.Join(directory, pluginID+"-latest.so")
	temporary := latest + fmt.Sprintf(".tmp-%d", time.Now().UnixNano())
	_ = os.Remove(temporary)
	if err := os.Symlink(filepath.Base(target), temporary); err != nil {
		return err
	}
	if err := os.Rename(temporary, latest); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
