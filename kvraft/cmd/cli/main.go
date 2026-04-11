package main

import (
	"flag"
	"fmt"
	"os"

	"kvraft/api"
	"kvraft/config"
	kvclient "kvraft/pkg/clerk"
)

func main() {
	cfgPath := flag.String("config", "config/cluster.json", "cluster JSON")
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: kvclient [--config path] get <key> | put <key> <value> [version]")
		os.Exit(2)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	addrs := make([]string, len(cfg.Nodes))
	for i, n := range cfg.Nodes {
		addrs[i] = n.Addr
	}

	ck, err := kvclient.NewClerk(addrs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer ck.Close()

	switch args[0] {
	case "get":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: kvclient get <key>")
			os.Exit(2)
		}
		v, ver, e := ck.Get(args[1])
		fmt.Printf("value=%q version=%d err=%s\n", v, ver, e)
		if e != api.OK && e != api.ErrNoKey {
			os.Exit(1)
		}
	case "put":
		if len(args) < 3 || len(args) > 4 {
			fmt.Fprintln(os.Stderr, "usage: kvclient put <key> <value> [version]")
			os.Exit(2)
		}
		var ver api.TVersion
		if len(args) == 4 {
			var u uint64
			if _, err := fmt.Sscanf(args[3], "%d", &u); err != nil {
				fmt.Fprintln(os.Stderr, "bad version")
				os.Exit(2)
			}
			ver = api.TVersion(u)
		}
		e := ck.Put(args[1], args[2], ver)
		fmt.Printf("err=%s\n", e)
		if e != api.OK {
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command", args[0])
		os.Exit(2)
	}
}
