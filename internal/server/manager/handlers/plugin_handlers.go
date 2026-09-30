package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-engine/pkg/utils"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

// ListPluginsHandler 列出所有插件
func (m *Manager) ListPluginsHandler(c *gin.Context) {
	// 检查是否需要描述符
	descriptors := c.Query("descriptors") == "true"

	if descriptors {
		// 扫描插件目录（确保获取最新插件）
		if err := m.pluginLoader.ScanPlugins(); err != nil {
			pkglog.Warn("Failed to scan plugins: %v", err)
		}

		// 所有插件通过插件加载器加载
		allPlugins := m.pluginLoader.ListPlugins()

		// 转换为 map（去重）
		pluginMap := make(map[string]sdk.PluginInfo)
		for _, p := range allPlugins {
			pluginMap[p.ID] = p
		}

		pluginList := make([]PluginDescriptorView, 0, len(pluginMap))
		for _, p := range pluginMap {
			pluginList = append(pluginList, describePlugin(p))
		}
		sort.Slice(pluginList, func(i, j int) bool {
			if pluginList[i].Type != pluginList[j].Type {
				return pluginList[i].Type < pluginList[j].Type
			}
			if pluginList[i].Name != pluginList[j].Name {
				return pluginList[i].Name < pluginList[j].Name
			}
			return pluginList[i].ID < pluginList[j].ID
		})

		pkglog.Info("ListPlugins: %d total plugins", len(pluginList))

		c.JSON(http.StatusOK, gin.H{
			"plugins": pluginList,
			"count":   len(pluginList),
		})
		return
	}

	// 返回插件元数据列表
	pluginClients := m.pluginLoader.ListPluginClients()
	metaMap := make(map[string]PluginMeta, len(pluginClients))
	for _, client := range pluginClients {
		meta := m.toPluginMeta(client)
		metaMap[meta.ID] = meta
	}

	installedMetas, err := m.listInstalledPluginMetas()
	if err != nil {
		pkglog.Warn("Failed to list installed plugins: %v", err)
	}
	for _, meta := range installedMetas {
		if _, exists := metaMap[meta.ID]; !exists {
			metaMap[meta.ID] = meta
		}
	}

	metas := make([]PluginMeta, 0, len(metaMap))
	for _, meta := range metaMap {
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool {
		if metas[i].Type != metas[j].Type {
			return metas[i].Type < metas[j].Type
		}
		if metas[i].Name != metas[j].Name {
			return metas[i].Name < metas[j].Name
		}
		return metas[i].ID < metas[j].ID
	})
	c.JSON(http.StatusOK, gin.H{
		"plugins": metas,
		"count":   len(metas),
	})
}

// GetPluginHandler 获取单个插件
func (m *Manager) GetPluginHandler(c *gin.Context) {
	name := c.Param("name")

	_, client, err := m.resolvePluginClient(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, m.toPluginMeta(client))
}

// UploadPluginHandler 上传插件
func (m *Manager) UploadPluginHandler(c *gin.Context) {
	if !m.preparePluginUpload(c) {
		return
	}
	m.installMu.Lock()
	defer m.installMu.Unlock()

	// 获取上传的文件
	file, err := c.FormFile("plugin")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin file is required"})
		return
	}
	uploadName, err := safeUploadName(file.Filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Step 1: Save to a temporary directory for verification
	tempDir, err := os.MkdirTemp("", "plugin-upload-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temp dir"})
		return
	}
	defer os.RemoveAll(tempDir) // Clean up

	tempPluginPath := filepath.Join(tempDir, uploadName)
	if err := c.SaveUploadedFile(file, tempPluginPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Make executable if needed
	if err := os.Chmod(tempPluginPath, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to secure uploaded plugin"})
		return
	}

	trusted, err := m.verifyTrustedArtifact(tempPluginPath, uploadName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("trust verification failed: %v", err)})
		return
	}

	// Step 2: Verify the plugin using Loader
	descriptor, err := m.pluginLoader.VerifyPlugin(tempPluginPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("verification failed: %v", err)})
		return
	}
	if descriptor.ID != trusted.ID || descriptor.Version != trusted.Version || string(descriptor.Type) != trusted.Type {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin identity does not match trusted manifest"})
		return
	}

	// Step 3: Determine destination based on Type
	targetSubDir := getPluginSubDir(descriptor.Type)
	if filepath.Base(filepath.Dir(filepath.FromSlash(trusted.Artifact))) != targetSubDir {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin category does not match trusted manifest"})
		return
	}

	// Create target dir
	finalDir := filepath.Join(m.pluginsDir, targetSubDir)
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create target dir"})
		return
	}

	finalPath := filepath.Join(finalDir, filepath.Base(filepath.FromSlash(trusted.Artifact)))
	if _, err := os.Lstat(finalPath); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "plugin artifact already exists; use upgrade"})
		return
	} else if !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to inspect target artifact"})
		return
	}

	// Step 4: Move file to final destination
	stagedFile, err := os.CreateTemp(finalDir, ".plugin-install-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create install staging file"})
		return
	}
	stagedPath := stagedFile.Name()
	_ = stagedFile.Close()
	defer os.Remove(stagedPath)
	if err := utils.CopyFile(tempPluginPath, stagedPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to install plugin: %v", err)})
		return
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to secure installed plugin"})
		return
	}
	if err := os.Rename(stagedPath, finalPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to commit plugin install: %v", err)})
		return
	}

	// Step 5: Load the plugin
	pluginClient, err := m.loadManagedPlugin(finalPath, descriptor.Type)
	if err != nil {
		_ = os.Remove(finalPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("loaded failed after install: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, m.toPluginMeta(pluginClient))
}

// EnablePluginHandler 启用插件
func (m *Manager) EnablePluginHandler(c *gin.Context) {
	name := c.Param("name")

	_, client, err := m.resolvePluginClient(name)
	if err == nil {
		// 如果插件未加载，重新加载
		if client.Client == nil || client.Client.Exited() {
			pluginPath := client.Path
			newClient, err := m.loadManagedPlugin(pluginPath, client.Info.Type)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			client = newClient
		}

		c.JSON(http.StatusOK, m.toPluginMeta(client))
		return
	}

	pluginPath, pathErr := m.resolvePluginPath(name)
	if pathErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	pluginInfo, verifyErr := m.pluginLoader.VerifyPlugin(pluginPath)
	if verifyErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": verifyErr.Error()})
		return
	}

	newClient, loadErr := m.loadManagedPlugin(pluginPath, pluginInfo.Type)
	if loadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": loadErr.Error()})
		return
	}

	c.JSON(http.StatusOK, m.toPluginMeta(newClient))
}

// DisablePluginHandler 禁用插件
func (m *Manager) DisablePluginHandler(c *gin.Context) {
	name := c.Param("name")

	pluginID, client, err := m.resolvePluginClient(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !m.ensurePluginNotReferenced(c, pluginID) {
		return
	}

	// 卸载插件
	if err := m.pluginLoader.UnloadPlugin(pluginID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 返回插件元数据（已禁用状态）
	meta := m.toPluginMeta(client)
	meta.Enabled = false
	c.JSON(http.StatusOK, meta)
}

// DeletePluginHandler 删除插件
func (m *Manager) DeletePluginHandler(c *gin.Context) {
	name := c.Param("name")

	pluginID, client, err := m.resolvePluginClient(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !m.ensurePluginNotReferenced(c, pluginID) {
		return
	}

	// 先卸载插件
	if err := m.pluginLoader.UnloadPlugin(pluginID); err != nil {
		pkglog.Warn("Plugin %s not loaded, continuing with file deletion", name)
	}

	if client.Path == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin file not found"})
		return
	}

	if err := os.Remove(client.Path); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	latestLink := filepath.Join(m.pluginsDir, getPluginSubDir(client.Info.Type), fmt.Sprintf("%s-latest.so", pluginID))
	if info, err := os.Lstat(latestLink); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(latestLink); err == nil {
			targetPath := target
			if !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(filepath.Dir(latestLink), targetPath)
			}
			if filepath.Clean(targetPath) == filepath.Clean(client.Path) {
				_ = os.Remove(latestLink)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "plugin deleted"})
}

type pluginRuleReference struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	LifecycleStatus string `json:"lifecycle_status,omitempty"`
}

type pluginConnectorResourceReference struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
}

// ensurePluginNotReferenced protects both deployed and draft rules. Disabling
// or deleting a referenced plugin would make later deployment, restart or rule
// editing fail even when the rule is not currently running.
func (m *Manager) ensurePluginNotReferenced(c *gin.Context, pluginID string) bool {
	if m.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rules database is unavailable"})
		return false
	}

	rules, err := m.db.ListRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to inspect plugin dependencies: %v", err)})
		return false
	}

	resources, err := m.db.ListConnectorResources()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to inspect connector dependencies: %v", err)})
		return false
	}
	resourceIDs := make(map[string]struct{})
	resourceReferences := make([]pluginConnectorResourceReference, 0)
	for _, resource := range resources {
		if resource == nil || resource.PluginID != pluginID {
			continue
		}
		resourceIDs[resource.ID] = struct{}{}
		resourceReferences = append(resourceReferences, pluginConnectorResourceReference{
			ID:      resource.ID,
			Name:    resource.Name,
			Enabled: resource.Enabled,
		})
	}

	references := make([]pluginRuleReference, 0)
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		for _, node := range rule.Nodes {
			if node == nil {
				continue
			}
			_, referencesConnector := resourceIDs[connectorReferenceFromConfig(node.Config)]
			if node.Plugin == pluginID || referencesConnector {
				references = append(references, pluginRuleReference{
					ID:              rule.ID,
					Name:            rule.Name,
					Enabled:         rule.Enabled,
					LifecycleStatus: rule.LifecycleStatus,
				})
				break
			}
		}
	}
	if len(references) == 0 && len(resourceReferences) == 0 {
		return true
	}

	sort.Slice(references, func(i, j int) bool {
		if references[i].Name != references[j].Name {
			return references[i].Name < references[j].Name
		}
		return references[i].ID < references[j].ID
	})
	sort.Slice(resourceReferences, func(i, j int) bool {
		if resourceReferences[i].Name != resourceReferences[j].Name {
			return resourceReferences[i].Name < resourceReferences[j].Name
		}
		return resourceReferences[i].ID < resourceReferences[j].ID
	})
	c.JSON(http.StatusConflict, gin.H{
		"error":               "plugin is referenced by existing rules or connector resources",
		"code":                "PLUGIN_IN_USE",
		"plugin_id":           pluginID,
		"rules":               references,
		"connector_resources": resourceReferences,
	})
	return false
}

func connectorReferenceFromConfig(config map[string]interface{}) string {
	if config == nil {
		return ""
	}
	value, _ := config["connector_ref"].(string)
	return strings.TrimSpace(value)
}

// UpgradePluginHandler 升级插件
func (m *Manager) UpgradePluginHandler(c *gin.Context) {
	if !m.preparePluginUpload(c) {
		return
	}
	m.installMu.Lock()
	defer m.installMu.Unlock()

	name := c.Param("name")

	pluginID, client, err := m.resolvePluginClient(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 获取上传的文件
	file, err := c.FormFile("plugin")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin file is required"})
		return
	}
	uploadName, err := safeUploadName(file.Filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tempDir, err := os.MkdirTemp("", "plugin-upgrade-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upgrade staging directory"})
		return
	}
	defer os.RemoveAll(tempDir)
	tempPluginPath := filepath.Join(tempDir, uploadName)
	if err := c.SaveUploadedFile(file, tempPluginPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := os.Chmod(tempPluginPath, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to secure uploaded plugin"})
		return
	}
	trusted, err := m.verifyTrustedArtifact(tempPluginPath, uploadName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("trust verification failed: %v", err)})
		return
	}
	descriptor, err := m.pluginLoader.VerifyPlugin(tempPluginPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("verification failed: %v", err)})
		return
	}
	if descriptor.ID != pluginID || descriptor.ID != trusted.ID || descriptor.Version != trusted.Version || descriptor.Type != client.Info.Type || string(descriptor.Type) != trusted.Type {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upgrade identity does not match installed plugin and trusted manifest"})
		return
	}
	if filepath.Base(filepath.Dir(filepath.FromSlash(trusted.Artifact))) != getPluginSubDir(client.Info.Type) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upgrade category does not match trusted manifest"})
		return
	}

	// 先在目标文件系统内完成 staging；只有验证全部通过后才卸载旧进程并原子切换。
	pluginDir := filepath.Join(m.pluginsDir, getPluginSubDir(client.Info.Type))
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create target dir"})
		return
	}
	pluginPath := filepath.Join(pluginDir, filepath.Base(filepath.FromSlash(trusted.Artifact)))
	if filepath.Clean(pluginPath) != filepath.Clean(client.Path) {
		if _, err := os.Lstat(pluginPath); err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "target plugin artifact already exists"})
			return
		} else if !os.IsNotExist(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to inspect target artifact"})
			return
		}
	}
	stagedFile, err := os.CreateTemp(pluginDir, ".plugin-upgrade-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upgrade staging file"})
		return
	}
	stagedPath := stagedFile.Name()
	_ = stagedFile.Close()
	defer os.Remove(stagedPath)
	if err := utils.CopyFile(tempPluginPath, stagedPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to stage upgrade: %v", err)})
		return
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to secure staged upgrade"})
		return
	}

	if err := m.pluginLoader.UnloadPlugin(pluginID); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("plugin is still in use: %v", err)})
		return
	}

	oldPath := client.Path
	backupPath := ""
	if filepath.Clean(oldPath) == filepath.Clean(pluginPath) {
		backupPath = oldPath + fmt.Sprintf(".rollback-%d", time.Now().UnixNano())
		if err := os.Rename(oldPath, backupPath); err != nil {
			_, _ = m.loadManagedPlugin(oldPath, client.Info.Type)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to prepare rollback artifact: %v", err)})
			return
		}
	}
	rollback := func() {
		_ = os.Remove(pluginPath)
		if backupPath != "" {
			_ = os.Rename(backupPath, oldPath)
		}
		if _, err := m.loadManagedPlugin(oldPath, client.Info.Type); err != nil {
			pkglog.Error("Failed to rollback plugin %s: %v", pluginID, err)
		}
	}
	if err := os.Rename(stagedPath, pluginPath); err != nil {
		rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to commit plugin upgrade: %v", err)})
		return
	}

	// Load new
	pluginClient, err := m.loadManagedPlugin(pluginPath, client.Info.Type)
	if err != nil {
		rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if backupPath != "" {
		_ = os.Remove(backupPath)
	}

	c.JSON(http.StatusOK, m.toPluginMeta(pluginClient))
}

// toPluginMeta 转换为插件元数据
func (m *Manager) toPluginMeta(client *plugin.PluginClient) PluginMeta {
	enabled := client.Client != nil && !client.Client.Exited()

	// 获取文件修改时间
	uploadedAt := ""
	if info, err := os.Stat(client.Path); err == nil {
		uploadedAt = info.ModTime().Format(time.RFC3339)
	}

	// 复制 PluginInfo
	pluginInfo := client.Info

	return PluginMeta{
		ID:          client.Info.ID,
		Name:        client.Name,
		Version:     client.Info.Version,
		Type:        string(client.Info.Type),
		Description: client.Info.Description,
		Enabled:     enabled,
		EngineAPI:   "",
		UploadedAt:  uploadedAt,
		Info:        &pluginInfo,
	}
}

func (m *Manager) listInstalledPluginMetas() ([]PluginMeta, error) {
	processedFiles := make(map[string]bool)
	metas := make([]PluginMeta, 0)

	err := filepath.Walk(m.pluginsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path)[0] == '.' {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".so" && ext != ".exe" && ext != ".dll" {
			return nil
		}

		realPath := path
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := os.Readlink(path)
			if err != nil {
				pkglog.Warn("Failed to resolve symlink %s: %v", path, err)
				return nil
			}
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(path), resolved)
			}
			realPath = resolved
		}

		realPath = filepath.Clean(realPath)
		if processedFiles[realPath] {
			return nil
		}
		processedFiles[realPath] = true

		pluginInfo, err := m.pluginLoader.VerifyPlugin(realPath)
		if err != nil {
			pkglog.Warn("Failed to verify plugin %s: %v", realPath, err)
			return nil
		}

		uploadedAt := ""
		if fileInfo, err := os.Stat(realPath); err == nil {
			uploadedAt = fileInfo.ModTime().Format(time.RFC3339)
		}

		infoCopy := *pluginInfo
		metas = append(metas, PluginMeta{
			ID:          pluginInfo.ID,
			Name:        pluginInfo.Name,
			Version:     pluginInfo.Version,
			Type:        string(pluginInfo.Type),
			Description: pluginInfo.Description,
			Enabled:     false,
			EngineAPI:   "",
			UploadedAt:  uploadedAt,
			Info:        &infoCopy,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return metas, nil
}

func (m *Manager) resolvePluginClient(identifier string) (string, *plugin.PluginClient, error) {
	for _, candidate := range pluginIdentifierVariants(identifier) {
		client, err := m.pluginLoader.GetPluginClient(candidate)
		if err == nil {
			return candidate, client, nil
		}
	}

	for _, pluginClient := range m.pluginLoader.ListPluginClients() {
		if pluginIdentifierMatches(pluginClient.Info.ID, identifier) ||
			pluginIdentifierMatches(pluginClient.Name, identifier) ||
			pluginIdentifierMatches(pluginClient.Info.Name, identifier) {
			return pluginClient.Info.ID, pluginClient, nil
		}
	}

	return "", nil, fmt.Errorf("plugin %s not found", identifier)
}

func (m *Manager) resolvePluginPath(identifier string) (string, error) {
	for _, candidate := range pluginIdentifierVariants(identifier) {
		if path, err := m.pluginLoader.ResolvePluginPath(candidate); err == nil {
			return path, nil
		}
	}

	for _, pluginClient := range m.pluginLoader.ListPluginClients() {
		if pluginIdentifierMatches(pluginClient.Info.ID, identifier) ||
			pluginIdentifierMatches(pluginClient.Name, identifier) ||
			pluginIdentifierMatches(pluginClient.Info.Name, identifier) {
			if path, err := m.pluginLoader.ResolvePluginPath(pluginClient.Info.ID); err == nil {
				return path, nil
			}
			if pluginClient.Path != "" {
				return pluginClient.Path, nil
			}
		}
	}

	return "", fmt.Errorf("plugin %s not found", identifier)
}

func pluginIdentifierVariants(identifier string) []string {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil
	}

	candidates := []string{trimmed}
	replacerPairs := [][2]string{
		{"-", "_"},
		{"_", "-"},
	}

	for _, pair := range replacerPairs {
		variant := strings.ReplaceAll(trimmed, pair[0], pair[1])
		if variant != trimmed {
			candidates = append(candidates, variant)
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		unique = append(unique, candidate)
	}
	return unique
}

func pluginIdentifierMatches(actual, requested string) bool {
	for _, candidate := range pluginIdentifierVariants(requested) {
		if actual == candidate {
			return true
		}
	}
	return false
}

// getPluginSubDir 根据插件类型返回子目录名
func getPluginSubDir(pluginType sdk.PluginType) string {
	switch pluginType {
	case sdk.PluginTypeSource:
		return "sources"
	case sdk.PluginTypeSink:
		return "sinks"
	case sdk.PluginTypeProcessor:
		return "processors"
	case sdk.PluginTypeUtility:
		return "utilities"
	case sdk.PluginTypeConnector:
		return "connectors"
	default:
		return "others"
	}
}

func (m *Manager) loadManagedPlugin(pluginPath string, pluginType sdk.PluginType) (*plugin.PluginClient, error) {
	if pluginType == sdk.PluginTypeConnector {
		return m.pluginLoader.LoadConnectorPlugin(pluginPath)
	}
	return m.pluginLoader.LoadPlugin(pluginPath)
}
