// Package log provides structured JSON logging
package log

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents log severity
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// Logger provides structured logging
type Logger struct {
	mu     *sync.Mutex
	w      io.Writer
	level  Level
	fields map[string]interface{}
}

// NewLogger creates a new logger
func NewLogger(w io.Writer, level Level) *Logger {
	if w == nil {
		w = os.Stdout
	}
	return &Logger{
		mu:     &sync.Mutex{},
		w:      w,
		level:  level,
		fields: make(map[string]interface{}),
	}
}

// logEntry represents a log entry
type logEntry struct {
	Timestamp string                 `json:"ts"`
	Level     string                 `json:"level"`
	Message   string                 `json:"msg"`
	Fields    map[string]interface{} `json:"-"`
}

// MarshalJSON flattens Fields into the top-level JSON object
func (e logEntry) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{}, 3+len(e.Fields))
	m["ts"] = e.Timestamp
	m["level"] = e.Level
	m["msg"] = e.Message
	for k, v := range e.Fields {
		m[k] = v
	}
	return json.Marshal(m)
}

// log writes a log entry
func (l *Logger) log(level Level, msg string, fields ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := logEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Message:   msg,
		Fields:    make(map[string]interface{}),
	}

	// Add base fields
	for k, v := range l.fields {
		entry.Fields[k] = v
	}

	// Add call-specific fields
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key, ok := fields[i].(string)
			if ok {
				val := fields[i+1]
				// Handle error type specially
				if err, ok := val.(error); ok {
					entry.Fields[key] = err.Error()
				} else {
					entry.Fields[key] = val
				}
			}
		}
	}

	data, _ := json.Marshal(entry)
	l.w.Write(data)
	l.w.Write([]byte("\n"))
}

// Debug logs at debug level
func (l *Logger) Debug(msg string, fields ...interface{}) {
	l.log(LevelDebug, msg, fields...)
}

// Info logs at info level
func (l *Logger) Info(msg string, fields ...interface{}) {
	l.log(LevelInfo, msg, fields...)
}

// Warn logs at warn level
func (l *Logger) Warn(msg string, fields ...interface{}) {
	l.log(LevelWarn, msg, fields...)
}

// Error logs at error level
func (l *Logger) Error(msg string, fields ...interface{}) {
	l.log(LevelError, msg, fields...)
}

// Fatal logs at fatal level and exits
func (l *Logger) Fatal(msg string, fields ...interface{}) {
	l.log(LevelFatal, msg, fields...)
	os.Exit(1)
}

// With returns a logger with additional fields
func (l *Logger) With(fields ...interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newFields := make(map[string]interface{})
	for k, v := range l.fields {
		newFields[k] = v
	}
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key, ok := fields[i].(string)
			if ok {
				newFields[key] = fields[i+1]
			}
		}
	}

	return &Logger{
		mu:     l.mu,
		w:      l.w,
		level:  l.level,
		fields: newFields,
	}
}
