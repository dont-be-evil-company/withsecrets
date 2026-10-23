package log

import (
	"log/slog"
	"os"

	"github.com/dont-be-evil-company/withsecrets/internal/lib/version"
)

var logLevel slog.Level = slog.LevelInfo

func SetLogLevel(level slog.Level) {
	logLevel = level
	slog.SetLogLoggerLevel(level)
}

func SetDebugMode(debug bool) {
	if debug {
		SetLogLevel(slog.LevelDebug)
	} else {
		SetLogLevel(slog.LevelInfo)
	}
}

func IsDebugMode() bool {
	return logLevel <= slog.LevelDebug
}

func NewLogger() *slog.Logger {
	// When running in a production environment,
	// set the log level to Error unless debug mode is enabled
	if version.VERSION != "" && logLevel > slog.LevelDebug {
		logLevel = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
}
