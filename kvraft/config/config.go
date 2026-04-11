package config

import (
	"encoding/json"
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
	sort.Slice(cfg.Nodes, func(i, j int) bool { return cfg.Nodes[i].ID < cfg.Nodes[j].ID })
	return &cfg, nil
}
