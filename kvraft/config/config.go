package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type Node struct {
	ID   int    `json:"id"`
	Addr string `json:"addr"`
}

type Config struct {
	Nodes []Node `json:"nodes"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	
	if len(cfg.Nodes) < 3 {
		return nil, fmt.Errorf("cluster must have at least 3 nodes, got %d", len(cfg.Nodes))
	}
	if len(cfg.Nodes)%2 == 0 {
		return nil, fmt.Errorf("cluster must have an odd number of nodes, got %d", len(cfg.Nodes))
	}

	sort.Slice(cfg.Nodes, func(i, j int) bool { return cfg.Nodes[i].ID < cfg.Nodes[j].ID })
	return &cfg, nil
}
