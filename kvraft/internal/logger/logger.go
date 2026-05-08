package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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
