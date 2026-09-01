// Package logger provides a single structured logging entrypoint used by
// every module. Handlers/services never call zerolog directly — they take
// a *logger.Logger so the sink (console for local, JSON for prod) and
// enrichment (request_id, user_id) stay centralized.
package logger

import (
	"context"
	"os"
	"time"

	"github.com/rs/zerolog"
)

type Logger struct {
	zl zerolog.Logger
}

type ctxKey struct{}

// New builds the root logger. level: debug|info|warn|error. format: json|console.
func New(level, format, appName, appVersion string) *Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano

	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	var w = os.Stdout
	var base zerolog.Logger
	if format == "console" {
		cw := zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339}
		base = zerolog.New(cw)
	} else {
		base = zerolog.New(w)
	}

	base = base.With().
		Timestamp().
		Str("service", appName).
		Str("version", appVersion).
		Logger()

	return &Logger{zl: base}
}

// WithContext attaches request-scoped fields (request_id, user_id) and
// returns a context carrying the enriched logger for downstream retrieval.
func (l *Logger) WithContext(ctx context.Context, fields map[string]interface{}) context.Context {
	sub := l.zl.With().Fields(fields).Logger()
	return context.WithValue(ctx, ctxKey{}, &Logger{zl: sub})
}

// FromContext retrieves the request-scoped logger, falling back to a
// no-op-safe root logger if none was attached (should not happen in
// normal request flow since middleware always injects one).
func FromContext(ctx context.Context, fallback *Logger) *Logger {
	if l, ok := ctx.Value(ctxKey{}).(*Logger); ok {
		return l
	}
	return fallback
}

func (l *Logger) Debug() *zerolog.Event { return l.zl.Debug() }
func (l *Logger) Info() *zerolog.Event  { return l.zl.Info() }
func (l *Logger) Warn() *zerolog.Event  { return l.zl.Warn() }
func (l *Logger) Error() *zerolog.Event { return l.zl.Error() }
func (l *Logger) Fatal() *zerolog.Event { return l.zl.Fatal() }

// Raw exposes the underlying zerolog.Logger for libraries needing it directly.
func (l *Logger) Raw() zerolog.Logger { return l.zl }
