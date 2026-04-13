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
func InitLogger(isProduction bool) *zap.Logger {
	var config zap.Config

	if isProduction {
		config = zap.NewProductionConfig()
		config.Encoding = "json"
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.Encoding = "console"
	}

	// Set level based on environment variable
	level := zap.InfoLevel
	if os.Getenv("RAFT_DEBUG") == "true" {
		level = zap.DebugLevel
	}
	config.Level = zap.NewAtomicLevelAt(level)

	l, err := config.Build()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	return l
}
