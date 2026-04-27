package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"kvraft/config"
	"kvraft/internal/kvserver"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return fallback
}

func main() {
	var (
		cfgPath      = flag.String("config", getEnv("CONFIG_PATH", "cluster.json"), "cluster JSON")
		id           = flag.Int("id", getEnvInt("NODE_ID", -1), "node id (must match config)")
		dataDir      = flag.String("datadir", getEnv("DATA_DIR", ""), "persistent state directory")
		maxRaftState = flag.Int("maxraftstate", getEnvInt("MAX_RAFT_STATE", -1), "snapshot when persist size exceeds this (-1 = disable)")
		isProd       = flag.Bool("production", getEnvBool("IS_PROD", false), "run in production mode (JSON logs)")
		isDebug      = flag.Bool("debug", getEnvBool("DEBUG", false), "enable debug logging")
		logPath      = flag.String("logpath", getEnv("LOG_PATH", ""), "path to save logs to a specific file")
	)
	flag.Parse()
	if *id < 0 || *dataDir == "" {
		fmt.Fprintln(os.Stderr, "usage: kvserver --id N --datadir DIR [--config path] [--maxraftstate N]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	node, err := kvserver.StartNode(cfg, *id, *dataDir, *maxRaftState, *isProd, *isDebug, *logPath)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	node.Stop()
}
