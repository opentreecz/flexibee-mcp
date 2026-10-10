// Package observability provides a minimal structured logger that writes
// exclusively to stderr.  It never logs sensitive credential values.
package observability

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// sensitiveKeySet holds lowercase key names whose values must be redacted.
var sensitiveKeySet = map[string]bool{
	"password":   true,
	"passwd":     true,
	"secret":     true,
	"token":      true,
	"credential": true,
	"auth":       true,
}

// Redact reports whether key is a sensitive key whose value must never be
// logged.
func Redact(key string) bool {
	return sensitiveKeySet[strings.ToLower(key)]
}

// Logger is a minimal structured logger.
type Logger struct {
	w io.Writer
}

// DefaultLogger writes to os.Stderr.
var DefaultLogger = New(os.Stderr)

// New creates a Logger that writes to w.
func New(w io.Writer) *Logger {
	return &Logger{w: w}
}

// Info logs a message at INFO level.
func (l *Logger) Info(msg string, kvs ...interface{}) {
	l.emit("INFO", msg, kvs)
}

// Warn logs a message at WARN level.
func (l *Logger) Warn(msg string, kvs ...interface{}) {
	l.emit("WARN", msg, kvs)
}

// Error logs a message at ERROR level.
func (l *Logger) Error(msg string, kvs ...interface{}) {
	l.emit("ERROR", msg, kvs)
}

// emit formats and writes one log line to l.w.
// Output format: time=<RFC3339> level=<LEVEL> msg="<msg>" [key=value ...]
func (l *Logger) emit(level, msg string, kvs []interface{}) {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "time=%s level=%s msg=%q",
		time.Now().UTC().Format(time.RFC3339Nano),
		level,
		msg,
	)
	for i := 0; i+1 < len(kvs); i += 2 {
		key := fmt.Sprintf("%v", kvs[i])
		if Redact(key) {
			fmt.Fprintf(sb, " %s=[REDACTED]", key)
		} else {
			fmt.Fprintf(sb, " %s=%v", key, kvs[i+1])
		}
	}
	sb.WriteByte('\n')
	// Best-effort write; ignore the error to avoid recursive logging.
	_, _ = fmt.Fprint(l.w, sb.String())
}
