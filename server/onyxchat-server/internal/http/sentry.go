package http

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InitSentry initializes the Sentry SDK if SENTRY_DSN is set and returns a
// logger with an additional core that forwards Error+ level zap entries to
// Sentry, plus a flush function that must be called before process exit.
//
// If SENTRY_DSN is unset, Sentry is disabled and the logger is returned
// unchanged so the binary runs fine without it configured (e.g. in dev).
func InitSentry(log *zap.Logger, env string) (*zap.Logger, func()) {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		log.Info("Sentry disabled (SENTRY_DSN not set)")
		return log, func() {}
	}

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      env,
		Release:          os.Getenv("SENTRY_RELEASE"),
		AttachStacktrace: true,
	}); err != nil {
		log.Error("failed to init Sentry, continuing without it", zap.Error(err))
		return log, func() {}
	}

	log.Info("Sentry initialized", zap.String("environment", env))

	augmented := log.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, &sentryCore{minLevel: zapcore.ErrorLevel})
	}))

	return augmented, func() { sentry.Flush(2 * time.Second) }
}

// skipSentryKey marks a log entry that should still go through the normal
// logging core but must never be forwarded to Sentry. Attach it via
// SkipSentry() to expected error-level entries (e.g. a failed readiness
// probe) that would otherwise burn the Sentry error quota.
const skipSentryKey = "skip_sentry"

// SkipSentry returns a zap field that suppresses Sentry forwarding for the
// log entry it's attached to, without affecting normal logging.
func SkipSentry() zap.Field {
	return zap.Bool(skipSentryKey, true)
}

// sentryCore is a zapcore.Core that forwards Error+ level entries to Sentry.
// It's teed alongside the normal logging core rather than replacing it.
type sentryCore struct {
	minLevel zapcore.Level
	fields   []zapcore.Field
}

func (c *sentryCore) Enabled(lvl zapcore.Level) bool { return lvl >= c.minLevel }

func (c *sentryCore) With(fields []zapcore.Field) zapcore.Core {
	merged := make([]zapcore.Field, 0, len(c.fields)+len(fields))
	merged = append(merged, c.fields...)
	merged = append(merged, fields...)
	return &sentryCore{minLevel: c.minLevel, fields: merged}
}

func (c *sentryCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *sentryCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	all := make([]zapcore.Field, 0, len(c.fields)+len(fields))
	all = append(all, c.fields...)
	all = append(all, fields...)

	var capturedErr error
	extra := make(map[string]any, len(all))
	for _, f := range all {
		if f.Key == skipSentryKey {
			if f.Type == zapcore.BoolType && f.Integer == 1 {
				return nil
			}
			continue
		}
		if f.Type == zapcore.ErrorType {
			if err, ok := f.Interface.(error); ok {
				capturedErr = err
				continue
			}
		}
		extra[f.Key] = fieldValue(f)
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentryLevel(ent.Level))
		if len(extra) > 0 {
			scope.SetContext("log_fields", extra)
		}
		if capturedErr != nil {
			sentry.CaptureException(capturedErr)
		} else {
			sentry.CaptureMessage(ent.Message)
		}
	})

	if ent.Level >= zapcore.FatalLevel {
		sentry.Flush(2 * time.Second)
	}

	return nil
}

func (c *sentryCore) Sync() error { return nil }

func sentryLevel(lvl zapcore.Level) sentry.Level {
	if lvl >= zapcore.FatalLevel {
		return sentry.LevelFatal
	}
	return sentry.LevelError
}

// fieldValue extracts a zap field's value via zap's own map encoder, so all
// field types (strings, ints, durations, etc.) are handled generically.
func fieldValue(f zapcore.Field) any {
	enc := zapcore.NewMapObjectEncoder()
	f.AddTo(enc)
	return enc.Fields[f.Key]
}
