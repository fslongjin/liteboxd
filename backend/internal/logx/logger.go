package logx

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultLevel         = "info"
	defaultFormat        = "json"
	defaultOutput        = "stdout"
	defaultFilePath      = "./logs/liteboxd.log"
	defaultMaxSizeMB     = 100
	defaultMaxBackups    = 7
	defaultMaxAgeDays    = 7
	defaultCompress      = true
	defaultAddSource     = false
	envLogLevel          = "LOG_LEVEL"
	envLogFormat         = "LOG_FORMAT"
	envLogOutput         = "LOG_OUTPUT"
	envLogFilePath       = "LOG_FILE_PATH"
	envLogFileMaxSizeMB  = "LOG_FILE_MAX_SIZE_MB"
	envLogFileMaxBackups = "LOG_FILE_MAX_BACKUPS"
	envLogFileMaxAgeDays = "LOG_FILE_MAX_AGE_DAYS"
)

type Config struct {
	Level       slog.Level
	Format      string
	Output      string
	FilePath    string
	MaxSizeMB   int
	MaxBackups  int
	MaxAgeDays  int
	Compress    bool
	AddSource   bool
	ServiceName string
}

func LoadConfig(serviceName string) Config {
	cfg := Config{
		Level:       parseLevel(getenv(envLogLevel, defaultLevel)),
		Format:      normalizeFormat(getenv(envLogFormat, defaultFormat)),
		Output:      normalizeOutput(getenv(envLogOutput, defaultOutput)),
		FilePath:    getenv(envLogFilePath, defaultFilePath),
		MaxSizeMB:   getenvInt(envLogFileMaxSizeMB, defaultMaxSizeMB),
		MaxBackups:  getenvInt(envLogFileMaxBackups, defaultMaxBackups),
		MaxAgeDays:  getenvInt(envLogFileMaxAgeDays, defaultMaxAgeDays),
		Compress:    defaultCompress,
		AddSource:   defaultAddSource,
		ServiceName: serviceName,
	}
	return cfg
}

func Init(serviceName string) (*slog.Logger, func() error, error) {
	cfg := LoadConfig(serviceName)
	writer, closer, err := buildWriter(cfg)
	if err != nil {
		return nil, nil, err
	}
	handler := buildHandler(cfg, writer)
	logger := slog.New(handler).With("service", cfg.ServiceName)
	slog.SetDefault(logger)

	return logger, closer, nil
}

func buildHandler(cfg Config, writer io.Writer) slog.Handler {
	options := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource,
	}
	if cfg.Format == "text" {
		return slog.NewTextHandler(writer, options)
	}
	return slog.NewJSONHandler(writer, options)
}

func buildWriter(cfg Config) (io.Writer, func() error, error) {
	useStdout := strings.Contains(cfg.Output, "stdout")
	useFile := strings.Contains(cfg.Output, "file")

	if !useStdout && !useFile {
		useStdout = true
	}

	writers := make([]io.Writer, 0, 2)
	var closers []io.Closer

	if useStdout {
		writers = append(writers, os.Stdout)
	}

	if useFile {
		logDir := filepath.Dir(cfg.FilePath)
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return nil, nil, err
		}
		rotator := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		}
		writers = append(writers, rotator)
		closers = append(closers, rotator)
	}

	closeFn := func() error {
		var lastErr error
		for _, c := range closers {
			if err := c.Close(); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}

	if len(writers) == 1 {
		return writers[0], closeFn, nil
	}
	return io.MultiWriter(writers...), closeFn, nil
}

func normalizeFormat(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "text":
		return "text"
	default:
		return "json"
	}
}

func normalizeOutput(v string) string {
	out := strings.ToLower(strings.TrimSpace(v))
	switch out {
	case "stdout", "file", "stdout,file":
		return out
	default:
		return defaultOutput
	}
}

func parseLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getenv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getenvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
