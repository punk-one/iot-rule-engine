package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	global   *logrus.Logger
	globalMu sync.RWMutex
)

func init() {
	global = newDefaultLogger()
}

// Config defines reusable logger configuration.
type Config struct {
	Level            string `yaml:"level" json:"level"`
	File             string `yaml:"file" json:"file"`
	MaxSize          int    `yaml:"max_size" json:"max_size"`
	MaxFiles         int    `yaml:"max_files" json:"max_files"`
	MaxBackups       int    `yaml:"max_backups" json:"max_backups"`
	Compress         bool   `yaml:"compress" json:"compress"`
	PluginDir        string `yaml:"plugin_dir" json:"plugin_dir"`
	PluginMaxSize    int    `yaml:"plugin_max_size" json:"plugin_max_size"`
	PluginMaxFiles   int    `yaml:"plugin_max_files" json:"plugin_max_files"`
	PluginMaxBackups int    `yaml:"plugin_max_backups" json:"plugin_max_backups"`
	PluginCompress   *bool  `yaml:"plugin_compress" json:"plugin_compress"`
}

func newDefaultLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	return logger
}

// Init applies logger configuration to the global logger instance.
func Init(cfg Config) error {
	logger := newDefaultLogger()

	level := strings.TrimSpace(strings.ToLower(cfg.Level))
	if level == "" {
		level = "info"
	}

	parsedLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid logging.level %q: %w", cfg.Level, err)
	}
	logger.SetLevel(parsedLevel)

	output, err := buildOutput(cfg)
	if err != nil {
		return err
	}
	logger.SetOutput(output)

	globalMu.Lock()
	global = logger
	globalMu.Unlock()

	pluginMgr, err := newPluginLogManager(cfg)
	if err != nil {
		return err
	}
	setPluginLogManager(pluginMgr)
	return nil
}

// Get returns the current global logger.
func Get() *logrus.Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

func buildOutput(cfg Config) (io.Writer, error) {
	if strings.TrimSpace(cfg.File) == "" {
		return os.Stdout, nil
	}

	logPath, err := expandPath(cfg.File)
	if err != nil {
		return nil, fmt.Errorf("resolve logging.file %q: %w", cfg.File, err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory for %s: %w", logPath, err)
	}

	maxSize := cfg.MaxSize
	if maxSize <= 0 {
		maxSize = 100
	}

	maxBackups := cfg.MaxBackups
	if maxBackups < 0 {
		maxBackups = 0
	}

	if cfg.MaxFiles > 0 {
		maxAllowedBackups := cfg.MaxFiles - 1
		if maxAllowedBackups < 0 {
			maxAllowedBackups = 0
		}
		if maxBackups == 0 || maxBackups > maxAllowedBackups {
			maxBackups = maxAllowedBackups
		}
	}

	return &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		Compress:   cfg.Compress,
	}, nil
}

func expandPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", nil
	}
	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if trimmed == "~" {
			return homeDir, nil
		}
		return filepath.Join(homeDir, trimmed[2:]), nil
	}
	return filepath.Abs(trimmed)
}

// Info logs an info message.
func Info(format string, values ...interface{}) {
	Get().Infof(format, values...)
}

// Error logs an error message.
func Error(format string, values ...interface{}) {
	Get().Errorf(format, values...)
}

// Debug logs a debug message.
func Debug(format string, values ...interface{}) {
	Get().Debugf(format, values...)
}

// Warn logs a warning message.
func Warn(format string, values ...interface{}) {
	Get().Warnf(format, values...)
}
