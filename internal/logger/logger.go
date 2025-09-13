package logger

import (
	"log/slog"
	"os"
	"strings"
)

func ParseLevel(s string) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(s))); err == nil {
		return level
	}
	return slog.LevelInfo
}

func SetupLogger(logLevel string) *slog.Logger {
	level := ParseLevel(logLevel)

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	}))
}
