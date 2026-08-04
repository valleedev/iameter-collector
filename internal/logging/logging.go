// Package logging provides a small leveled logger that never prints
// secrets. Callers pass raw values; Redact* helpers must be used explicitly
// on anything that could contain a token, full statusLine JSON, or other
// sensitive payload before it reaches a log call.
package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelSilent
)

func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "silent", "none", "off":
		return LevelSilent
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "SILENT"
	}
}

// Logger writes leveled lines to an io.Writer (stderr by default so stdout
// stays clean for statusline/JSON output).
type Logger struct {
	out   io.Writer
	level Level
}

func New(out io.Writer, level Level) *Logger {
	if out == nil {
		out = os.Stderr
	}
	return &Logger{out: out, level: level}
}

func Default(level Level) *Logger {
	return New(os.Stderr, level)
}

func (l *Logger) log(lvl Level, format string, args ...any) {
	if lvl < l.level {
		return
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.out, "%s [%s] %s\n", ts, lvl.String(), msg)
}

func (l *Logger) Debug(format string, args ...any) { l.log(LevelDebug, format, args...) }
func (l *Logger) Info(format string, args ...any)  { l.log(LevelInfo, format, args...) }
func (l *Logger) Warn(format string, args ...any)  { l.log(LevelWarn, format, args...) }
func (l *Logger) Error(format string, args ...any) { l.log(LevelError, format, args...) }

// RedactToken returns a short, non-reversible preview safe for logs:
// first 4 chars + "..." + length. Never log full tokens.
func RedactToken(token string) string {
	if token == "" {
		return "<empty>"
	}
	n := len(token)
	prefix := token
	if n > 4 {
		prefix = token[:4]
	}
	return fmt.Sprintf("%s...(%d chars)", prefix, n)
}
