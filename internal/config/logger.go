package config

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

// RingLogBuffer retains recent log lines in fixed memory (zero unbounded growth).
type RingLogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}

var GlobalLogBuffer = &RingLogBuffer{
	entries: make([]LogEntry, 0, 300),
	maxSize: 300,
}

func (b *RingLogBuffer) Add(level, msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level,
		Message:   msg,
	}
	if len(b.entries) >= b.maxSize {
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = entry
	} else {
		b.entries = append(b.entries, entry)
	}
}

func (b *RingLogBuffer) Recent() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]LogEntry, len(b.entries))
	copy(out, b.entries)
	return out
}

type teeHandler struct {
	inner slog.Handler
}

func (h *teeHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	GlobalLogBuffer.Add(r.Level.String(), r.Message)
	return h.inner.Handle(ctx, r)
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &teeHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	return &teeHandler{inner: h.inner.WithGroup(name)}
}

// InitLogger initializes rotating file logging with bounded disk and memory usage.
func InitLogger(filename string, alsoStdout bool) io.Closer {
	_ = os.MkdirAll(LogsDir(), 0750)
	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(LogsDir(), filename),
		MaxSize:    10, // MB
		MaxBackups: 5,
		MaxAge:     30, // days
		Compress:   true,
	}
	var writer io.Writer = rotator
	if alsoStdout {
		writer = io.MultiWriter(rotator, os.Stdout)
	}
	jsonHandler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(&teeHandler{inner: jsonHandler}))
	return rotator
}

// SafeGo launches a goroutine protected against panics so the Windows Service never crashes.
func SafeGo(component string, fn func()) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("recovered from panic in background worker",
					"component", component,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
