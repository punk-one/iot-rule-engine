package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	pluginrpc "github.com/punk-one/iot-rule-engine/internal/core/plugin/rpc"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

// PluginClient 封装插件客户端
type PluginClient struct {
	Name                string
	Path                string
	Client              *plugin.Client
	RPCClient           sdk.Plugin
	ConnectorRPCClient  *sdk.ConnectorRPCClient
	Info                sdk.PluginInfo
	Checksum            string // SHA256 校验和
	LastRestart         time.Time
	RestartCount        int
	ConsecutiveFailures int
	CircuitOpenUntil    time.Time
	CanonicalPath       string
	Protocol            plugin.ClientProtocol
	ActiveInstances     int
	mutex               sync.RWMutex
	owner               *PluginClient
	releaseOnce         sync.Once
	releaseErr          error
}

// RestartPolicy controls plugin crash recovery behavior.
type RestartPolicy struct {
	BaseDelay        time.Duration
	MaxDelay         time.Duration
	MaxRestarts      int
	JitterRatio      float64
	FailureThreshold int
	CircuitCooldown  time.Duration
}

// Loader 管理所有插件
// 职责：
//   - 插件进程拉起 / 停止
//   - RPC 连接管理
//   - 崩溃自动重启（可配置）
//   - 插件白名单管理
//   - 心跳与健康检查
type Loader struct {
	pluginsDir string
	plugins    map[string]*PluginClient // plugin id -> client
	whitelist  map[string]bool          // 插件白名单（可选）
	mutex      sync.RWMutex
	acquireMu  sync.Mutex
	policy     RestartPolicy
}

// NewLoader 创建新的插件加载器
func NewLoader(pluginsDir string) *Loader {
	if pluginsDir == "" {
		pluginsDir = "./plugins"
	}

	return &Loader{
		pluginsDir: pluginsDir,
		plugins:    make(map[string]*PluginClient),
		whitelist:  make(map[string]bool),
		policy: RestartPolicy{
			BaseDelay:        2 * time.Second,
			MaxDelay:         30 * time.Second,
			MaxRestarts:      5,
			JitterRatio:      0.2,
			FailureThreshold: 5,
			CircuitCooldown:  30 * time.Second,
		},
	}
}

// SetRestartPolicy overrides loader restart policy.
func (l *Loader) SetRestartPolicy(policy RestartPolicy) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if policy.BaseDelay <= 0 {
		policy.BaseDelay = 2 * time.Second
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = 30 * time.Second
	}
	if policy.MaxRestarts <= 0 {
		policy.MaxRestarts = 5
	}
	if policy.JitterRatio <= 0 || policy.JitterRatio > 1 {
		policy.JitterRatio = 0.2
	}
	if policy.FailureThreshold <= 0 {
		policy.FailureThreshold = 5
	}
	if policy.CircuitCooldown <= 0 {
		policy.CircuitCooldown = 30 * time.Second
	}

	l.policy = policy
}

// SetWhitelist 设置插件白名单
func (l *Loader) SetWhitelist(plugins []string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.whitelist = make(map[string]bool)
	for _, name := range plugins {
		l.whitelist[name] = true
	}
}

// calculateChecksum 计算文件的 SHA256 校验和
func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// LoadPlugin 加载一个插件
func (l *Loader) LoadPlugin(pluginPath string) (*PluginClient, error) {
	pluginClient, regularErr := l.SpawnPlugin(pluginPath)
	if regularErr != nil {
		var connectorErr error
		pluginClient, connectorErr = l.SpawnConnectorPlugin(pluginPath)
		if connectorErr != nil {
			return nil, fmt.Errorf("failed to load plugin %s: regular plugin error: %v; connector plugin error: %w", pluginPath, regularErr, connectorErr)
		}
	}

	return l.registerLoadedPlugin(pluginClient), nil
}

// LoadConnectorPlugin 加载一个 connector 插件。
func (l *Loader) LoadConnectorPlugin(pluginPath string) (*PluginClient, error) {
	pluginClient, err := l.SpawnConnectorPlugin(pluginPath)
	if err != nil {
		return nil, err
	}
	return l.registerLoadedPlugin(pluginClient), nil
}

func (l *Loader) registerLoadedPlugin(pluginClient *PluginClient) *PluginClient {
	pluginInfo := pluginClient.Info

	l.mutex.Lock()
	// 清理同名插件
	if old, exists := l.plugins[pluginInfo.ID]; exists {
		old.mutex.RLock()
		activeInstances := old.ActiveInstances
		old.mutex.RUnlock()
		if activeInstances > 0 {
			l.mutex.Unlock()
			pluginClient.Client.Kill()
			pkglog.Warn("Plugin %s has %d active instances; keeping current process", pluginInfo.ID, activeInstances)
			return old
		}
		replace, reason := shouldReplaceLoadedPlugin(old, pluginClient)
		if !replace {
			l.mutex.Unlock()
			if pluginClient.ConnectorRPCClient != nil {
				_ = pluginClient.ConnectorRPCClient.Stop()
			}
			pluginClient.Client.Kill()
			pkglog.Warn("Plugin %s already loaded, keeping existing artifact %s (version %s): %s",
				pluginInfo.ID, old.Path, old.Info.Version, reason)
			return old
		}
		pkglog.Warn("Plugin %s already loaded, replacing %s (version %s) with %s (version %s): %s",
			pluginInfo.ID, old.Path, old.Info.Version, pluginClient.Path, pluginInfo.Version, reason)
		if old.ConnectorRPCClient != nil {
			_ = old.ConnectorRPCClient.Stop()
		}
		old.Client.Kill()
	}
	l.plugins[pluginInfo.ID] = pluginClient
	l.mutex.Unlock()

	pkglog.Info("Loaded plugin: %s (type: %s, version: %s, checksum: %s)",
		pluginInfo.Name, pluginInfo.Type, pluginInfo.Version, pluginClient.Checksum[:8])
	return pluginClient
}

func shouldReplaceLoadedPlugin(existing, candidate *PluginClient) (bool, string) {
	if existing == nil {
		return true, "no existing plugin loaded"
	}
	if candidate == nil {
		return false, "candidate plugin is nil"
	}
	if existing.Client != nil && existing.Client.Exited() {
		return true, "existing plugin process has exited"
	}

	existingLatest := isLatestArtifactPath(existing.Path)
	candidateLatest := isLatestArtifactPath(candidate.Path)
	if existingLatest != candidateLatest {
		if candidateLatest {
			return true, "prefer -latest artifact"
		}
		return false, "existing artifact is marked as -latest"
	}

	if cmp := comparePluginVersion(candidate.Info.Version, existing.Info.Version); cmp != 0 {
		if cmp > 0 {
			return true, "prefer higher plugin version"
		}
		return false, "existing plugin version is newer"
	}

	if existing.Path == candidate.Path {
		return false, "same artifact already loaded"
	}

	return false, "existing plugin has equal precedence"
}

func isLatestArtifactPath(path string) bool {
	return strings.HasSuffix(path, "-latest.so")
}

func comparePluginVersion(left, right string) int {
	leftParts := parseVersionParts(left)
	rightParts := parseVersionParts(right)
	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}
	for i := 0; i < maxLen; i++ {
		var lv, rv int
		if i < len(leftParts) {
			lv = leftParts[i]
		}
		if i < len(rightParts) {
			rv = rightParts[i]
		}
		switch {
		case lv > rv:
			return 1
		case lv < rv:
			return -1
		}
	}
	return 0
}

func parseVersionParts(version string) []int {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if version == "" {
		return nil
	}
	rawParts := strings.Split(version, ".")
	parts := make([]int, 0, len(rawParts))
	for _, raw := range rawParts {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			parts = append(parts, 0)
			continue
		}
		digits := raw
		if idx := strings.IndexFunc(raw, func(r rune) bool {
			return r < '0' || r > '9'
		}); idx >= 0 {
			digits = raw[:idx]
		}
		if digits == "" {
			parts = append(parts, 0)
			continue
		}
		value, err := strconv.Atoi(digits)
		if err != nil {
			parts = append(parts, 0)
			continue
		}
		parts = append(parts, value)
	}
	return parts
}

// SpawnPlugin 启动一个插件进程但不注册到 Loader
// 用于支持 Multi-Instance 运行时
func (l *Loader) SpawnPlugin(pluginPath string) (*PluginClient, error) {
	// 检查文件是否存在
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin file not found: %s", pluginPath)
	}

	// 强制校验：计算校验和失败时阻止加载
	checksum, err := calculateChecksum(pluginPath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum for %s: %w", pluginPath, err)
	}

	// 获取插件名称（用于日志）
	pluginName := filepath.Base(pluginPath)
	logSink := pkglog.NewPluginProcessLogSink()

	// 创建插件客户端（带日志隧道）
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: handshakeConfig(),
		Plugins: map[string]plugin.Plugin{
			"plugin":         &sdk.PluginRPC{},
			"plugin_factory": &sdk.PluginRPC{},
		},
		Cmd: exec.Command(pluginPath),
		AllowedProtocols: []plugin.Protocol{
			plugin.ProtocolNetRPC,
		},
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   fmt.Sprintf("plugin-%s", pluginName),
			Output: logSink,
			Level:  hclog.Info,
		}),
		SyncStdout: logSink,
		SyncStderr: logSink,
	})

	// 连接到 RPC 客户端
	clientProtocol, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	// 获取插件接口
	raw, err := clientProtocol.Dispense("plugin")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}

	// 类型断言为 PluginRPCClient
	pluginRPCClient, ok := raw.(*sdk.PluginRPCClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin does not implement Plugin interface")
	}

	// 获取插件信息
	pluginInfo := pluginRPCClient.Info()

	// 检查白名单
	l.mutex.RLock()
	whitelistEnabled := len(l.whitelist) > 0
	inWhitelist := l.whitelist[pluginInfo.ID]
	l.mutex.RUnlock()

	if whitelistEnabled && !inWhitelist {
		client.Kill()
		return nil, fmt.Errorf("plugin %s is not in whitelist", pluginInfo.ID)
	}

	// 创建包装器以支持 Plugin 接口
	pluginWrapper := &pluginWrapper{
		rpcClient: pluginRPCClient,
		info:      pluginInfo,
		broker:    pluginRPCClient.Broker, // 保存 MuxBroker 用于注册 RPC 服务
	}
	canonicalPath, err := canonicalPluginPath(pluginPath)
	if err != nil {
		client.Kill()
		return nil, err
	}

	pluginClient := &PluginClient{
		Name:          pluginInfo.Name,
		Path:          pluginPath,
		Client:        client,
		RPCClient:     pluginWrapper,
		Info:          pluginInfo,
		Checksum:      checksum,
		CanonicalPath: canonicalPath,
		Protocol:      clientProtocol,
		LastRestart:   time.Now(),
		RestartCount:  0,
	}

	return pluginClient, nil
}

// pluginWrapper 包装 RPC 客户端以实现 Plugin 接口
type pluginWrapper struct {
	rpcClient *sdk.PluginRPCClient
	info      sdk.PluginInfo
	broker    *plugin.MuxBroker // 用于注册 RPC 服务
}

func (w *pluginWrapper) Info() sdk.PluginInfo {
	return w.info
}

func (w *pluginWrapper) Init(cfg sdk.PluginConfig) error {
	return w.rpcClient.Init(cfg)
}

func (w *pluginWrapper) Start(ctx sdk.RuntimeContext) error {
	var brokerID uint32
	// 从 RuntimeContext 获取 EngineRPCServer（在 Engine 端创建并注入 dispatcher）
	if w.broker != nil {
		bridgeCtx, ok := ctx.(sdk.EngineBridgeRuntimeContext)
		if !ok {
			pkglog.Error("RuntimeContext does not implement EngineBridgeRuntimeContext for node %s", ctx.NodeID())
			return fmt.Errorf("RuntimeContext does not provide Engine bridge capabilities")
		}

		// EngineRPCServer 应该在 Engine 端创建，而不是在 Plugin 端创建
		var engineRPC *pluginrpc.EngineRPCServer
		engineRPCServer := bridgeCtx.GetEngineRPCServer()
		if engineRPCServer != nil {
			if erpc, ok := engineRPCServer.(*pluginrpc.EngineRPCServer); ok {
				engineRPC = erpc
				pkglog.Debug("Retrieved EngineRPCServer from RuntimeContext for node %s", ctx.NodeID())
			} else {
				pkglog.Error("Failed to cast EngineRPCServer from RuntimeContext for node %s", ctx.NodeID())
				return fmt.Errorf("invalid EngineRPCServer type from RuntimeContext")
			}
		} else {
			pkglog.Error("EngineRPCServer is nil in RuntimeContext for node %s", ctx.NodeID())
			return fmt.Errorf("EngineRPCServer not found in RuntimeContext")
		}

		bridgeCtx.SavePluginRPCClient(w.rpcClient) // 保存 PluginRPCClient
		pkglog.Debug("Saved PluginRPCClient for node %s via RuntimeContext callback", ctx.NodeID())

		// 每个规则节点使用唯一 broker ID，避免共享进程中的反向 RPC 连接串线。
		brokerID = w.broker.NextId()
		// 外部插件通过 EngineRPC 发布事件和记录日志
		if engineRPC != nil {
			go func() {
				conn, err := w.broker.Accept(brokerID)
				if err != nil {
					pkglog.Error("Failed to accept connection for Engine RPC: %v", err)
					return
				}
				pkglog.Debug("Engine RPC connection accepted")
				rpcServer := rpc.NewServer()
				if err := rpcServer.RegisterName("Engine", engineRPC); err != nil {
					pkglog.Error("Failed to register Engine RPC: %v", err)
					return
				}
				pkglog.Debug("Engine RPC service registered")
				rpcServer.ServeConn(conn)
			}()
		} else {
			pkglog.Error("EngineRPC is nil, cannot register Engine RPC service")
		}
	}

	// 通过 RPC 传递 RuleID 和 NodeID，在插件进程内创建 RuntimeContext
	args := &sdk.StartArgs{
		RuleID:                ctx.RuleID(),
		NodeID:                ctx.NodeID(),
		Health:                sdk.DefaultHealthOptions(),
		ResourceEventsEnabled: ctx.ResourceEvents() != nil,
		BrokerID:              brokerID,
	}
	return w.rpcClient.Start(args)
}

// AcquirePluginInstance dispenses one isolated rule-node object from the single
// process owned by the plugin artifact. The process stays loaded until Loader
// unload/shutdown; stopping a rule releases only its object.
func (l *Loader) AcquirePluginInstance(pluginPath string) (*PluginClient, error) {
	canonicalPath, err := canonicalPluginPath(pluginPath)
	if err != nil {
		return nil, err
	}

	l.acquireMu.Lock()
	defer l.acquireMu.Unlock()

	owner := l.findRegularProcess(canonicalPath)
	if owner == nil {
		owner, err = l.LoadPlugin(pluginPath)
		if err != nil {
			return nil, err
		}
	}
	if owner.Info.Type == sdk.PluginTypeConnector || owner.Protocol == nil {
		return nil, fmt.Errorf("plugin %s is not a regular factory plugin", pluginPath)
	}

	raw, err := owner.Protocol.Dispense("plugin_factory")
	if err != nil {
		return nil, fmt.Errorf("plugin %s does not support sdk.ServeFactory: %w", owner.Info.ID, err)
	}
	rpcClient, ok := raw.(*sdk.PluginRPCClient)
	if !ok {
		return nil, fmt.Errorf("plugin %s factory returned invalid RPC client %T", owner.Info.ID, raw)
	}
	info := rpcClient.Info()
	if info.ID == "" || info.ID != owner.Info.ID {
		_ = rpcClient.Client.Close()
		return nil, fmt.Errorf("plugin factory identity mismatch: process=%q instance=%q", owner.Info.ID, info.ID)
	}

	owner.mutex.Lock()
	owner.ActiveInstances++
	owner.mutex.Unlock()
	return &PluginClient{
		Name:          info.Name,
		Path:          owner.Path,
		Client:        owner.Client,
		RPCClient:     &pluginWrapper{rpcClient: rpcClient, info: info, broker: rpcClient.Broker},
		Info:          info,
		Checksum:      owner.Checksum,
		CanonicalPath: owner.CanonicalPath,
		Protocol:      owner.Protocol,
		owner:         owner,
	}, nil
}

// ReleasePluginInstance stops one rule-node object without killing the shared
// operating-system process. It is safe to call repeatedly.
func (l *Loader) ReleasePluginInstance(instance *PluginClient) error {
	if instance == nil {
		return nil
	}
	instance.releaseOnce.Do(func() {
		if instance.RPCClient != nil {
			instance.releaseErr = instance.RPCClient.Stop()
		}
		if instance.owner != nil {
			instance.owner.mutex.Lock()
			if instance.owner.ActiveInstances > 0 {
				instance.owner.ActiveInstances--
			}
			instance.owner.mutex.Unlock()
		}
	})
	return instance.releaseErr
}

func (l *Loader) findRegularProcess(canonicalPath string) *PluginClient {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	for _, candidate := range l.plugins {
		if candidate.Info.Type != sdk.PluginTypeConnector && candidate.CanonicalPath == canonicalPath {
			return candidate
		}
	}
	return nil
}

func canonicalPluginPath(pluginPath string) (string, error) {
	absPath, err := filepath.Abs(pluginPath)
	if err != nil {
		return "", fmt.Errorf("resolve absolute plugin path %s: %w", pluginPath, err)
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve plugin symlink %s: %w", pluginPath, err)
	}
	return filepath.Clean(resolved), nil
}

func (w *pluginWrapper) OnEvent(e sdk.Event) error {
	// 通过 RPC 调用插件的 OnEvent 方法
	return w.rpcClient.ReceiveEvent(e)
}

func (w *pluginWrapper) OnEvents(events []sdk.Event) error {
	// 通过 RPC 调用插件的 OnEvents 方法
	return w.rpcClient.ReceiveEvents(events)
}

func (w *pluginWrapper) Stop() error {
	return w.rpcClient.Stop()
}

// VerifyPlugin 验证插件文件是否合法
func (l *Loader) VerifyPlugin(pluginPath string) (*sdk.PluginInfo, error) {
	// 检查文件是否存在
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin file not found: %s", pluginPath)
	}

	pluginInfo, regularErr := l.verifyRegularPlugin(pluginPath)
	if regularErr == nil {
		return pluginInfo, nil
	}

	pluginInfo, connectorErr := l.verifyConnectorPlugin(pluginPath)
	if connectorErr == nil {
		return pluginInfo, nil
	}

	return nil, fmt.Errorf("verification failed for %s: regular plugin error: %v; connector plugin error: %w", pluginPath, regularErr, connectorErr)
}

func (l *Loader) verifyRegularPlugin(pluginPath string) (*sdk.PluginInfo, error) {
	pluginName := filepath.Base(pluginPath)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: handshakeConfig(),
		Plugins: map[string]plugin.Plugin{
			"plugin":         &sdk.PluginRPC{},
			"plugin_factory": &sdk.PluginRPC{},
		},
		Cmd: exec.Command(pluginPath),
		AllowedProtocols: []plugin.Protocol{
			plugin.ProtocolNetRPC,
		},
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   fmt.Sprintf("verify-%s", pluginName),
			Output: os.Stdout,
			Level:  hclog.Warn,
		}),
	})
	defer client.Kill()

	clientProtocol, err := client.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	raw, err := clientProtocol.Dispense("plugin")
	if err != nil {
		return nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}

	pluginRPCClient, ok := raw.(*sdk.PluginRPCClient)
	if !ok {
		return nil, fmt.Errorf("plugin does not implement Plugin interface")
	}

	pluginInfo := pluginRPCClient.Info()
	if _, err := calculateChecksum(pluginPath); err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}
	return &pluginInfo, nil
}

func (l *Loader) verifyConnectorPlugin(pluginPath string) (*sdk.PluginInfo, error) {
	pluginName := filepath.Base(pluginPath)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: handshakeConfig(),
		Plugins: map[string]plugin.Plugin{
			"connector": &sdk.ConnectorRPC{},
		},
		Cmd: exec.Command(pluginPath),
		AllowedProtocols: []plugin.Protocol{
			plugin.ProtocolNetRPC,
		},
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   fmt.Sprintf("verify-connector-%s", pluginName),
			Output: os.Stdout,
			Level:  hclog.Warn,
		}),
	})
	defer client.Kill()

	clientProtocol, err := client.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to connector plugin: %w", err)
	}

	raw, err := clientProtocol.Dispense("connector")
	if err != nil {
		return nil, fmt.Errorf("failed to dispense connector plugin: %w", err)
	}

	connectorRPCClient, ok := raw.(*sdk.ConnectorRPCClient)
	if !ok {
		return nil, fmt.Errorf("plugin does not implement ConnectorPlugin interface")
	}

	pluginInfo := connectorRPCClient.Info()
	if _, err := calculateChecksum(pluginPath); err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}
	return &pluginInfo, nil
}

// UnloadPlugin 卸载插件
func (l *Loader) UnloadPlugin(id string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	pluginClient, exists := l.plugins[id]
	if !exists {
		return fmt.Errorf("plugin %s not found", id)
	}
	pluginClient.mutex.RLock()
	activeInstances := pluginClient.ActiveInstances
	pluginClient.mutex.RUnlock()
	if activeInstances > 0 {
		return fmt.Errorf("plugin %s still has %d active rule instances", id, activeInstances)
	}

	// 关闭插件客户端
	if pluginClient.ConnectorRPCClient != nil {
		_ = pluginClient.ConnectorRPCClient.Stop()
	}
	pluginClient.Client.Kill()
	delete(l.plugins, id)

	pkglog.Info("Unloaded plugin: %s", id)
	return nil
}

// GetPlugin 获取插件（带健康检查）
func (l *Loader) GetPlugin(id string) (sdk.Plugin, error) {
	l.mutex.RLock()
	pluginClient, exists := l.plugins[id]
	policy := l.policy
	l.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin %s not found", id)
	}
	if pluginClient.Info.Type == sdk.PluginTypeConnector || pluginClient.RPCClient == nil {
		return nil, fmt.Errorf("plugin %s is a connector plugin and cannot run as a DAG node", id)
	}

	// 检查插件是否已退出
	if pluginClient.Client.Exited() {
		pkglog.Warn("Plugin %s has exited, attempting to reload", id)

		// 检查重启次数
		pluginClient.mutex.RLock()
		restartCount := pluginClient.RestartCount
		circuitOpenUntil := pluginClient.CircuitOpenUntil
		pluginClient.mutex.RUnlock()

		if !circuitOpenUntil.IsZero() && time.Now().Before(circuitOpenUntil) {
			return nil, fmt.Errorf("plugin %s circuit breaker open until %s", id, circuitOpenUntil.Format(time.RFC3339))
		}

		if restartCount >= policy.MaxRestarts {
			return nil, fmt.Errorf("plugin %s has exceeded max restart count (%d)", id, policy.MaxRestarts)
		}

		// 触发重载
		reloaded, err := l.ReloadPlugin(id)
		if err != nil {
			return nil, fmt.Errorf("failed to reload plugin %s: %w", id, err)
		}

		return reloaded.RPCClient, nil
	}

	return pluginClient.RPCClient, nil
}

// ReloadPlugin 重新加载插件
func (l *Loader) ReloadPlugin(id string) (*PluginClient, error) {
	l.mutex.Lock()
	pluginClient, exists := l.plugins[id]
	policy := l.policy
	if !exists {
		l.mutex.Unlock()
		return nil, fmt.Errorf("plugin %s not found", id)
	}
	pluginClient.mutex.RLock()
	activeInstances := pluginClient.ActiveInstances
	pluginClient.mutex.RUnlock()
	if activeInstances > 0 {
		l.mutex.Unlock()
		return nil, fmt.Errorf("plugin %s has %d active rule instances and cannot be reloaded", id, activeInstances)
	}

	// 增加重启计数
	pluginClient.mutex.Lock()
	pluginClient.RestartCount++
	restartCount := pluginClient.RestartCount
	pluginPath := pluginClient.Path
	pluginClient.mutex.Unlock()
	l.mutex.Unlock()

	// 指数退避 + 抖动
	delay := computeBackoffDelay(policy, restartCount)
	if elapsed := time.Since(pluginClient.LastRestart); elapsed < delay {
		time.Sleep(delay - elapsed)
	}

	pkglog.Info("Reloading plugin %s (attempt %d/%d, delay=%s)", id, restartCount, policy.MaxRestarts, delay)

	// 关闭旧客户端
	pluginClient.Client.Kill()

	// 重新加载
	newClient, err := l.LoadPlugin(pluginPath)
	if err != nil {
		pluginClient.mutex.Lock()
		pluginClient.ConsecutiveFailures++
		if pluginClient.ConsecutiveFailures >= policy.FailureThreshold {
			pluginClient.CircuitOpenUntil = time.Now().Add(policy.CircuitCooldown)
			pkglog.Warn("Plugin %s circuit breaker opened for %s after %d consecutive failures",
				id, policy.CircuitCooldown, pluginClient.ConsecutiveFailures)
		}
		pluginClient.mutex.Unlock()
		return nil, err
	}

	// 更新重启信息
	newClient.mutex.Lock()
	newClient.RestartCount = 0
	newClient.LastRestart = time.Now()
	newClient.ConsecutiveFailures = 0
	newClient.CircuitOpenUntil = time.Time{}
	newClient.mutex.Unlock()

	// 验证校验和是否改变
	if newClient.Checksum != pluginClient.Checksum {
		pkglog.Warn("Plugin %s checksum changed: %s -> %s", id,
			pluginClient.Checksum[:8], newClient.Checksum[:8])
	}

	return newClient, nil
}

func computeBackoffDelay(policy RestartPolicy, restartCount int) time.Duration {
	if restartCount < 1 {
		restartCount = 1
	}
	exponent := math.Pow(2, float64(restartCount-1))
	delay := time.Duration(float64(policy.BaseDelay) * exponent)
	if delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}

	// jitter in range [-ratio, +ratio]
	jitterFactor := 1 + (rand.New(rand.NewSource(time.Now().UnixNano())).Float64()*2-1)*policy.JitterRatio
	if jitterFactor < 0 {
		jitterFactor = 0
	}
	return time.Duration(float64(delay) * jitterFactor)
}

// ListPlugins 列出所有已加载的插件
func (l *Loader) ListPlugins() []sdk.PluginInfo {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	infos := make([]sdk.PluginInfo, 0, len(l.plugins))
	for _, pluginClient := range l.plugins {
		infos = append(infos, pluginClient.Info)
	}

	return infos
}

// ListPluginClients 列出所有插件客户端
func (l *Loader) ListPluginClients() []*PluginClient {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	clients := make([]*PluginClient, 0, len(l.plugins))
	for _, client := range l.plugins {
		clients = append(clients, client)
	}

	return clients
}

// IsPluginLoaded 检查插件是否已加载
func (l *Loader) IsPluginLoaded(id string) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	client, exists := l.plugins[id]
	if !exists {
		return false
	}

	// 检查插件是否已退出
	return client.Client != nil && !client.Client.Exited()
}

// GetPluginClient 获取插件客户端
func (l *Loader) GetPluginClient(id string) (*PluginClient, error) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	client, exists := l.plugins[id]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", id)
	}

	return client, nil
}

// GetConnectorPluginClient 获取连接器插件客户端
func (l *Loader) GetConnectorPluginClient(id string) (*PluginClient, error) {
	client, err := l.GetPluginClient(id)
	if err != nil {
		return nil, err
	}
	if client.Info.Type != sdk.PluginTypeConnector || client.ConnectorRPCClient == nil {
		return nil, fmt.Errorf("plugin %s is not a connector plugin", id)
	}
	return client, nil
}

// ResolvePluginPath 解析插件 ID 到文件路径
// 优先使用已加载插件的路径，否则在插件目录中查找匹配的文件
func (l *Loader) ResolvePluginPath(pluginID string) (string, error) {
	// 1. 首先尝试获取已加载的插件客户端（如果插件已通过 ScanPlugins 加载）
	l.mutex.RLock()
	if client, exists := l.plugins[pluginID]; exists && client.Path != "" {
		path := client.Path
		l.mutex.RUnlock()
		return path, nil
	}
	l.mutex.RUnlock()

	// 2. 在插件目录中查找匹配的插件文件
	// 查找顺序：{pluginID}-latest.so, {pluginID}-*.so
	var candidates []string
	candidates = append(candidates, filepath.Join(l.pluginsDir, "sources", fmt.Sprintf("%s-latest.so", pluginID)))
	candidates = append(candidates, filepath.Join(l.pluginsDir, "sinks", fmt.Sprintf("%s-latest.so", pluginID)))
	candidates = append(candidates, filepath.Join(l.pluginsDir, "processors", fmt.Sprintf("%s-latest.so", pluginID)))
	candidates = append(candidates, filepath.Join(l.pluginsDir, "connectors", fmt.Sprintf("%s-latest.so", pluginID)))

	// 尝试直接匹配
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// 3. 通配符搜索 {pluginID}-*.so
	searchDirs := []string{
		filepath.Join(l.pluginsDir, "sources"),
		filepath.Join(l.pluginsDir, "sinks"),
		filepath.Join(l.pluginsDir, "processors"),
		filepath.Join(l.pluginsDir, "connectors"),
	}

	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			// 检查是否以 {pluginID}- 开头且以 .so 结尾
			if strings.HasPrefix(name, pluginID+"-") && strings.HasSuffix(name, ".so") {
				return filepath.Join(dir, name), nil
			}
		}
	}

	return "", fmt.Errorf("plugin file not found for plugin ID: %s", pluginID)
}

// ScanPlugins 扫描插件目录并加载所有插件 (Recursive)
func (l *Loader) ScanPlugins() error {
	// 确保目录存在
	if err := os.MkdirAll(l.pluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %w", err)
	}

	loadedCount := 0
	failedCount := 0
	processedFiles := make(map[string]bool) // 用于跟踪已处理的文件（避免重复处理符号链接）

	err := filepath.Walk(l.pluginsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Skip hidden files
		if filepath.Base(path)[0] == '.' {
			return nil
		}

		// 只处理 .so 文件（Linux）或 .exe/.dll 文件（Windows）
		ext := filepath.Ext(path)
		if ext != ".so" && ext != ".exe" && ext != ".dll" {
			return nil
		}

		// 处理符号链接：解析真实路径，避免重复加载
		realPath := path
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := os.Readlink(path)
			if err != nil {
				pkglog.Warn("Failed to resolve symlink %s: %v", path, err)
				return nil
			}
			// 如果是相对路径，转换为绝对路径
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(path), resolved)
			}
			realPath = resolved

			// 如果真实文件已经处理过，跳过符号链接
			if processedFiles[realPath] {
				return nil
			}
		}

		// 检查文件是否已处理
		if processedFiles[realPath] {
			return nil
		}
		processedFiles[realPath] = true

		// 检查文件是否存在且可执行
		realInfo, err := os.Stat(realPath)
		if err != nil {
			pkglog.Warn("Failed to stat plugin file %s: %v", realPath, err)
			return nil
		}

		// Check executable (跳过符号链接的可执行检查，直接检查真实文件)
		if realInfo.Mode()&0111 == 0 && os.Getenv("GOOS") != "windows" {
			// 尝试添加执行权限（可选）
			pkglog.Debug("Plugin file %s is not executable, attempting to chmod", realPath)
		}

		pkglog.Debug("Loading plugin: %s", realPath)
		load := l.LoadPlugin
		if filepath.Base(filepath.Dir(realPath)) == "connectors" {
			load = l.LoadConnectorPlugin
		}
		if _, err := load(realPath); err != nil {
			failedCount++
			pkglog.Error("Failed to load plugin %s: %v", realPath, err)
			return nil // 继续扫描其他插件
		}

		loadedCount++
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to scan plugins directory: %w", err)
	}

	pkglog.Info("Scanned plugins directory: %d loaded, %d failed", loadedCount, failedCount)
	return nil
}

// Close 关闭所有插件
func (l *Loader) Close() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	for id, pluginClient := range l.plugins {
		pluginClient.Client.Kill()
		pkglog.Info("Closed plugin: %s", id)
	}

	l.plugins = make(map[string]*PluginClient)
}
