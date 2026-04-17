package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InitLogger initializes a high-performance Zap logger.
// If isProduction is true, it uses JSON encoding.
// Otherwise, it uses Console encoding (colorized).
// Panics if initialization fails.
func InitLogger(isProduction bool, isDebug bool, logPath string) *zap.Logger {
	var config zap.Config

	if isProduction {
		config = zap.NewProductionConfig()
		config.Encoding = "json"
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.Encoding = "console"
	}

	// Set level
	level := zap.InfoLevel
	if isDebug || os.Getenv("RAFT_DEBUG") == "true" {
		level = zap.DebugLevel
	}

	if logPath != "" {
		config.OutputPaths = []string{"stderr", logPath}
		config.ErrorOutputPaths = []string{"stderr", logPath}
	}
	config.Level = zap.NewAtomicLevelAt(level)

	l, err := config.Build()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	return l
}
