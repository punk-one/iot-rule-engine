package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

func (l *Loader) SpawnConnectorPlugin(pluginPath string) (*PluginClient, error) {
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("connector plugin file not found: %s", pluginPath)
	}

	checksum, err := calculateChecksum(pluginPath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum for %s: %w", pluginPath, err)
	}

	pluginName := filepath.Base(pluginPath)
	logSink := pkglog.NewPluginProcessLogSink()

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: handshakeConfig(),
		Plugins: map[string]goplugin.Plugin{
			"connector": &sdk.ConnectorRPC{},
		},
		Cmd: exec.Command(pluginPath),
		AllowedProtocols: []goplugin.Protocol{
			goplugin.ProtocolNetRPC,
		},
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   fmt.Sprintf("connector-%s", pluginName),
			Output: logSink,
			Level:  hclog.Info,
		}),
		SyncStdout: logSink,
		SyncStderr: logSink,
	})

	clientProtocol, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to connect to connector plugin: %w", err)
	}

	raw, err := clientProtocol.Dispense("connector")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to dispense connector plugin: %w", err)
	}

	connectorRPCClient, ok := raw.(*sdk.ConnectorRPCClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin does not implement ConnectorPlugin interface")
	}

	pluginInfo := connectorRPCClient.Info()

	l.mutex.RLock()
	whitelistEnabled := len(l.whitelist) > 0
	inWhitelist := l.whitelist[pluginInfo.ID]
	l.mutex.RUnlock()

	if whitelistEnabled && !inWhitelist {
		client.Kill()
		return nil, fmt.Errorf("plugin %s is not in whitelist", pluginInfo.ID)
	}

	return &PluginClient{
		Name:               pluginInfo.Name,
		Path:               pluginPath,
		Client:             client,
		ConnectorRPCClient: connectorRPCClient,
		Info:               pluginInfo,
		Checksum:           checksum,
		LastRestart:        time.Now(),
	}, nil
}
