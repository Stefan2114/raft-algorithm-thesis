package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"kvraft/config"
	"kvraft/internal/kvserver"
)

func main() {
	var (
		cfgPath      = flag.String("config", "config/cluster.json", "cluster JSON")
		id           = flag.Int("id", -1, "node id (must match config)")
		dataDir      = flag.String("datadir", "", "persistent state directory")
		maxRaftState = flag.Int("maxraftstate", -1, "snapshot when persist size exceeds this (-1 = disable)")
		isProd       = flag.Bool("production", false, "run in production mode (JSON logs)")
		isDebug      = flag.Bool("debug", false, "enable debug logging")
		logPath      = flag.String("logpath", "", "path to save logs to a specific file")
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
