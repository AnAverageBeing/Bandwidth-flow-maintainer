// Package logger provides structured, leveled logging with console and file output,
// rotation, and JSON/text format support.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// Level represents log severity.
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

var levelStrings = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
}

func (l Level) String() string {
	if s, ok := levelStrings[l]; ok {
		return s
	}
	return "UNKNOWN"
}

// Logger is a structured, thread-safe logger.
type Logger struct {
	mu       sync.Mutex
	level    Level
	console  bool
	file     *os.File
	format   string // "text" or "json"
	stdLog   *log.Logger
	fileLog  *log.Logger
	filePath string
	maxSize  int64
}

// Config holds logger initialization parameters.
type Config struct {
	Level     string
	Console   bool
	File      string
	Format    string // "text" or "json"
	MaxSizeMB int
}

// New creates a new Logger from configuration.
func New(cfg Config) (*Logger, error) {
	l := &Logger{
		console:  cfg.Console,
		format:   cfg.Format,
		filePath: cfg.File,
		maxSize:  int64(cfg.MaxSizeMB) * 1024 * 1024,
	}

	switch cfg.Level {
	case "debug":
		l.level = DEBUG
	case "info":
		l.level = INFO
	case "warn":
		l.level = WARN
	case "error":
		l.level = ERROR
	default:
		l.level = INFO
	}

	if l.format == "" {
		l.format = "text"
	}

	if cfg.Console {
		l.stdLog = log.New(os.Stdout, "", 0)
	}

	if cfg.File != "" {
		if err := os.MkdirAll(dirname(cfg.File), 0755); err != nil {
			return nil, fmt.Errorf("logger: mkdir %s: %w", dirname(cfg.File), err)
		}
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("logger: open %s: %w", cfg.File, err)
		}
		l.file = f
		l.fileLog = log.New(f, "", 0)
	}

	return l, nil
}

// Debug logs a debug-level message.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs an info-level message.
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs a warning-level message.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs an error-level message.
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	line := l.formatLine(level, msg)

	if l.console && l.stdLog != nil {
		l.stdLog.Println(line)
	}
	if l.fileLog != nil {
		l.fileLog.Println(line)
		l.maybeRotate()
	}
}

func (l *Logger) formatLine(level Level, msg string) string {
	ts := time.Now().Format(time.RFC3339)

	if l.format == "json" {
		// Minimal structured JSON log line
		return fmt.Sprintf(`{"time":"%s","level":"%s","msg":%q}`, ts, level.String(), msg)
	}

	return fmt.Sprintf("%s [%s] %s", ts, level.String(), msg)
}

func (l *Logger) maybeRotate() {
	if l.file == nil || l.maxSize <= 0 {
		return
	}
	info, err := l.file.Stat()
	if err != nil {
		return
	}
	if info.Size() < l.maxSize {
		return
	}

	// Simple rotation: close current, rename, reopen
	l.file.Close()
	rotated := l.filePath + "." + time.Now().Format("20060102-150405")
	os.Rename(l.filePath, rotated)

	f, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	l.file = f
	l.fileLog = log.New(f, "", 0)
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Writer returns an io.Writer that writes at INFO level. Useful for stdlib log integration.
func (l *Logger) Writer(level Level) io.Writer {
	return &logWriter{l: l, level: level}
}

type logWriter struct {
	l     *Logger
	level Level
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.l.log(w.level, "%s", string(p))
	return len(p), nil
}

func dirname(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}
