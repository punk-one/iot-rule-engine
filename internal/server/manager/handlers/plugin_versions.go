package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/pkg/utils"
)

var safeRegistryID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)
var safeRegistryVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)

func (m *Manager) cacheVerifiedRegistryArtifact(item registryCatalogItem, artifact, bundle string) error {
	if !safeRegistryID.MatchString(item.Plugin.Plugin) || !safeRegistryVersion.MatchString(item.Plugin.Version) {
		return fmt.Errorf("invalid plugin cache identity")
	}
	dir := filepath.Join(m.pluginsDir, ".verified", item.Plugin.Plugin, item.Plugin.Version)
	if raw, err := os.ReadFile(filepath.Join(dir, "record.json")); err == nil {
		var previous registryCatalogItem
		if json.Unmarshal(raw, &previous) != nil || previous.Plugin.SHA256 != item.Plugin.SHA256 {
			return fmt.Errorf("cached plugin version is immutable")
		}
		return verifyFileSHA256(filepath.Join(dir, "plugin"), item.Plugin.SHA256)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), ".version-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := utils.CopyFile(artifact, filepath.Join(stage, "plugin")); err != nil {
		return err
	}
	if err := utils.CopyFile(bundle, filepath.Join(stage, "bundle.json")); err != nil {
		return err
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, "record.json"), raw, 0600); err != nil {
		return err
	}
	return os.Rename(stage, dir)
}

func (m *Manager) PluginVersionsHandler(c *gin.Context) {
	id := c.Param("name")
	if !safeRegistryID.MatchString(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plugin ID"})
		return
	}
	paths, _ := filepath.Glob(filepath.Join(m.pluginsDir, ".verified", id, "*", "record.json"))
	items := make([]registryCatalogItem, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var item registryCatalogItem
		if json.Unmarshal(raw, &item) == nil {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Plugin.Version < items[j].Plugin.Version })
	c.JSON(http.StatusOK, gin.H{"versions": items})
}

func (m *Manager) RollbackPluginHandler(c *gin.Context) {
	id := c.Param("name")
	var req struct {
		Version string `json:"version"`
	}
	if !safeRegistryID.MatchString(id) || c.ShouldBindJSON(&req) != nil || !safeRegistryVersion.MatchString(req.Version) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rollback request"})
		return
	}
	if !m.installPolicy.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "plugin installation is disabled"})
		return
	}
	m.installMu.Lock()
	defer m.installMu.Unlock()
	dir := filepath.Join(m.pluginsDir, ".verified", id, req.Version)
	raw, err := os.ReadFile(filepath.Join(dir, "record.json"))
	var item registryCatalogItem
	if err != nil || json.Unmarshal(raw, &item) != nil || item.Plugin.Plugin != id || item.Plugin.Version != req.Version {
		c.JSON(http.StatusNotFound, gin.H{"error": "verified version not cached"})
		return
	}
	if item.Plugin.OS != runtime.GOOS || item.Plugin.Arch != runtime.GOARCH || item.Plugin.SDK != supportedRegistrySDK {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "cached plugin is incompatible"})
		return
	}
	if _, err = safeUploadName(item.Plugin.Filename); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid cached filename"})
		return
	}
	artifact, bundle := filepath.Join(dir, "plugin"), filepath.Join(dir, "bundle.json")
	if err = verifyFileSHA256(artifact, item.Plugin.SHA256); err == nil {
		err = m.verifyRegistrySignature(c.Request.Context(), artifact, bundle)
	}
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if err = os.Chmod(artifact, 0700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	info, err := m.pluginLoader.VerifyPlugin(artifact)
	if err != nil || info.ID != id || info.Version != strings.TrimPrefix(item.Plugin.Version, "v") || string(info.Type) != item.Plugin.Type {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "cached plugin identity mismatch"})
		return
	}
	client, status, err := m.commitRegistryArtifact(artifact, item.Plugin.Filename, info)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, m.toPluginMeta(client))
}
