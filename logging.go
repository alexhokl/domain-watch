package main

import (
	"io"
	"log/slog"
)

// newLogger writes JSON in the shape Cloud Logging understands: severity,
// message and timestamp at the top level. Without the renaming every entry
// would be ingested at default severity and the alert filter on severity
// would never match.
//
// See https://cloud.google.com/logging/docs/structured-logging
func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: replaceAttrForGCP,
	}))
}

func replaceAttrForGCP(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return attr
	}
	switch attr.Key {
	case slog.LevelKey:
		attr.Key = "severity"
		if lvl, ok := attr.Value.Any().(slog.Level); ok {
			attr.Value = slog.StringValue(severity(lvl))
		}
	case slog.MessageKey:
		attr.Key = "message"
	case slog.TimeKey:
		attr.Key = "timestamp"
	}
	return attr
}

func severity(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARNING"
	case l >= levelNotice:
		return "NOTICE"
	case l >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}
