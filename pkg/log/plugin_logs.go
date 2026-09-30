package log

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"gopkg.in/natefinch/lumberjack.v2"
)

const maxPluginBootstrapBuffer = 1 << 20

var (
	pluginManager   *pluginLogManager
	pluginManagerMu sync.RWMutex
)

type pluginLogManager struct {
	dir        string
	maxSize    int
	maxBackups int
	compress   bool

	mu      sync.Mutex
	writers map[string]*lumberjack.Logger
}

func newPluginLogManager(cfg Config) (*pluginLogManager, error) {
	dir, err := resolvePluginLogDir(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve logging.plugin_dir %q: %w", cfg.PluginDir, err)
	}
	return &pluginLogManager{
		dir:        dir,
		maxSize:    pluginMaxSize(cfg),
		maxBackups: pluginMaxBackups(cfg),
		compress:   pluginCompress(cfg),
		writers:    make(map[string]*lumberjack.Logger),
	}, nil
}

func setPluginLogManager(manager *pluginLogManager) {
	pluginManagerMu.Lock()
	old := pluginManager
	pluginManager = manager
	pluginManagerMu.Unlock()
	if old != nil {
		old.Close()
	}
}

func getPluginLogManager() *pluginLogManager {
	pluginManagerMu.RLock()
	manager := pluginManager
	pluginManagerMu.RUnlock()
	if manager != nil {
		return manager
	}

	pluginManagerMu.Lock()
	defer pluginManagerMu.Unlock()
	if pluginManager != nil {
		return pluginManager
	}

	manager, err := newPluginLogManager(Config{})
	if err != nil {
		return &pluginLogManager{
			dir:        filepath.Join(".", "logs", "rules"),
			maxSize:    100,
			maxBackups: 3,
			writers:    make(map[string]*lumberjack.Logger),
		}
	}
	pluginManager = manager
	return pluginManager
}

func (m *pluginLogManager) Writer(logKey string) (io.Writer, error) {
	safeLogKey := sanitizePluginLogName(logKey)

	m.mu.Lock()
	defer m.mu.Unlock()

	if writer, exists := m.writers[safeLogKey]; exists {
		return writer, nil
	}

	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create plugin log directory %s: %w", m.dir, err)
	}

	logPath := filepath.Join(m.dir, safeLogKey+".log")
	writer := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    m.maxSize,
		MaxBackups: m.maxBackups,
		Compress:   m.compress,
	}
	m.writers[safeLogKey] = writer
	return writer, nil
}

func (m *pluginLogManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, writer := range m.writers {
		_ = writer.Close()
		delete(m.writers, key)
	}
}

func resolvePluginLogDir(cfg Config) (string, error) {
	dir := strings.TrimSpace(cfg.PluginDir)
	if dir == "" {
		dir = filepath.Join(".", "logs", "rules")
	}
	return expandPath(dir)
}

func pluginMaxSize(cfg Config) int {
	if cfg.PluginMaxSize > 0 {
		return cfg.PluginMaxSize
	}
	if cfg.MaxSize > 0 {
		return cfg.MaxSize
	}
	return 100
}

func pluginMaxBackups(cfg Config) int {
	maxBackups := cfg.PluginMaxBackups
	if maxBackups <= 0 {
		switch {
		case cfg.MaxBackups > 0:
			maxBackups = cfg.MaxBackups
		default:
			maxBackups = 3
		}
	}

	maxFiles := pluginMaxFiles(cfg)
	if maxFiles > 0 {
		maxAllowedBackups := maxFiles - 1
		if maxAllowedBackups < 0 {
			maxAllowedBackups = 0
		}
		if maxBackups == 0 || maxBackups > maxAllowedBackups {
			maxBackups = maxAllowedBackups
		}
	}

	return maxBackups
}

func pluginMaxFiles(cfg Config) int {
	if cfg.PluginMaxFiles > 0 {
		return cfg.PluginMaxFiles
	}
	if cfg.MaxFiles > 0 {
		return cfg.MaxFiles
	}
	return 5
}

func pluginCompress(cfg Config) bool {
	if cfg.PluginCompress != nil {
		return *cfg.PluginCompress
	}
	return cfg.Compress
}

func sanitizePluginLogName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "unknown-plugin"
	}

	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}

	safe := strings.Trim(builder.String(), ".-")
	if safe == "" {
		return "unknown-plugin"
	}
	return safe
}

type PluginProcessLogSink struct {
	mu      sync.Mutex
	writers map[string]io.Writer
	buffer  bytes.Buffer
	dropped bool
}

func NewPluginProcessLogSink() *PluginProcessLogSink {
	return &PluginProcessLogSink{
		writers: make(map[string]io.Writer),
	}
}

func (s *PluginProcessLogSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	writers := make([]io.Writer, 0, len(s.writers))
	for _, writer := range s.writers {
		writers = append(writers, writer)
	}
	if len(writers) == 0 {
		remaining := maxPluginBootstrapBuffer - s.buffer.Len()
		switch {
		case remaining <= 0:
			s.dropped = true
		case len(p) <= remaining:
			_, _ = s.buffer.Write(p)
		default:
			_, _ = s.buffer.Write(p[:remaining])
			s.dropped = true
		}
		s.mu.Unlock()
		return len(p), nil
	}
	s.mu.Unlock()
	for _, writer := range writers {
		if _, err := writer.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (s *PluginProcessLogSink) BindRuleID(ruleID string) error {
	writer, err := getPluginLogManager().Writer(ruleID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if _, exists := s.writers[ruleID]; exists {
		s.mu.Unlock()
		return nil
	}
	pending := append([]byte(nil), s.buffer.Bytes()...)
	dropped := s.dropped
	s.writers[ruleID] = writer
	shouldFlushPending := len(s.writers) == 1
	if shouldFlushPending {
		s.buffer.Reset()
		s.dropped = false
	}
	s.mu.Unlock()

	if shouldFlushPending && len(pending) > 0 {
		if _, writeErr := writer.Write(pending); writeErr != nil {
			return writeErr
		}
	}
	if shouldFlushPending && dropped {
		_, _ = io.WriteString(writer, time.Now().Format("2006-01-02 15:04:05")+" [WARN] plugin bootstrap logs truncated before rule ID was bound\n")
	}
	return nil
}

type pluginSDKLogger struct {
	mu       sync.Mutex
	writer   io.Writer
	ruleID   string
	pluginID string
	nodeID   string
}

func NewPluginSDKLogger(ruleID, pluginID, nodeID string) sdk.Logger {
	writer, err := getPluginLogManager().Writer(ruleID)
	if err != nil {
		writer = os.Stdout
	}
	return &pluginSDKLogger{
		writer:   writer,
		ruleID:   ruleID,
		pluginID: pluginID,
		nodeID:   nodeID,
	}
}

func (l *pluginSDKLogger) Debug(msg string, fields ...sdk.Field) { l.write("DEBUG", msg, fields...) }
func (l *pluginSDKLogger) Info(msg string, fields ...sdk.Field)  { l.write("INFO", msg, fields...) }
func (l *pluginSDKLogger) Warn(msg string, fields ...sdk.Field)  { l.write("WARN", msg, fields...) }
func (l *pluginSDKLogger) Error(msg string, fields ...sdk.Field) { l.write("ERROR", msg, fields...) }

func (l *pluginSDKLogger) LogBatch(level sdk.LogLevel, messages []string, fields []map[string]interface{}) {
	for i, msg := range messages {
		fieldList := make([]sdk.Field, 0)
		if i < len(fields) {
			for key, value := range fields[i] {
				fieldList = append(fieldList, sdk.Field{Key: key, Value: value})
			}
		}
		switch level {
		case sdk.LogLevelDebug:
			l.Debug(msg, fieldList...)
		case sdk.LogLevelInfo:
			l.Info(msg, fieldList...)
		case sdk.LogLevelWarn:
			l.Warn(msg, fieldList...)
		case sdk.LogLevelError:
			l.Error(msg, fieldList...)
		default:
			l.Info(msg, fieldList...)
		}
	}
}

func (l *pluginSDKLogger) write(level, msg string, fields ...sdk.Field) {
	var builder strings.Builder
	builder.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	builder.WriteString(" [")
	builder.WriteString(level)
	builder.WriteString("] ")
	builder.WriteString(msg)
	if l.ruleID != "" {
		builder.WriteString(" rule_id=")
		builder.WriteString(sanitizePluginLogName(l.ruleID))
	}
	if l.pluginID != "" {
		builder.WriteString(" plugin_id=")
		builder.WriteString(sanitizePluginLogName(l.pluginID))
	}
	if l.nodeID != "" {
		builder.WriteString(" node_id=")
		builder.WriteString(l.nodeID)
	}
	for _, field := range fields {
		if strings.TrimSpace(field.Key) == "" {
			continue
		}
		builder.WriteByte(' ')
		builder.WriteString(field.Key)
		builder.WriteByte('=')
		builder.WriteString(fmt.Sprint(field.Value))
	}
	builder.WriteByte('\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = io.WriteString(l.writer, builder.String())
}
