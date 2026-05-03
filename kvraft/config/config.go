package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
)

type Node struct {
	ID   int    `json:"id"`
	Addr string `json:"addr"`
}

type Config struct {
	Nodes               []Node `json:"nodes"`
	ElectionTimeoutMin  int    `json:"-"`
	ElectionTimeoutRand int    `json:"-"`
	HeartbeatTimeout    int    `json:"-"`
	SubmitTimeout       int    `json:"-"`
	BaseDelay           int    `json:"-"`
	MaxDelay            int    `json:"-"`
	MinConnectTimeout   int    `json:"-"`
	MetricsPortBase     int    `json:"-"`
}

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

func GetEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return fallback
}

func (cfg *Config) PopulateDefaults() {
	cfg.ElectionTimeoutMin = 600
	cfg.ElectionTimeoutRand = 400
	cfg.HeartbeatTimeout = 100
	cfg.SubmitTimeout = 10
	cfg.BaseDelay = 100
	cfg.MaxDelay = 3
	cfg.MinConnectTimeout = 2
	cfg.MetricsPortBase = 8080
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

	cfg.PopulateDefaults()

	// Override with environment variables
	if v, ok := os.LookupEnv("RAFT_ELECTION_TIMEOUT_MIN"); ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.ElectionTimeoutMin = i
		}
	}
	if v, ok := os.LookupEnv("RAFT_ELECTION_TIMEOUT_RAND"); ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.ElectionTimeoutRand = i
		}
	}
	if v, ok := os.LookupEnv("RAFT_HEARTBEAT_TIMEOUT"); ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.HeartbeatTimeout = i
		}
	}
	if v, ok := os.LookupEnv("RSM_SUBMIT_TIMEOUT"); ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.SubmitTimeout = i
		}
	}
	if v, ok := os.LookupEnv("GRPC_BASE_DELAY"); ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.BaseDelay = i
		}
	}
	if v, ok := os.LookupEnv("GRPC_MAX_DELAY"); ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.MaxDelay = i
		}
	}
	if v, ok := os.LookupEnv("GRPC_MIN_CONNECT_TIMEOUT"); ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.MinConnectTimeout = i
		}
	}
	if v, ok := os.LookupEnv("METRICS_PORT_BASE"); ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.MetricsPortBase = i
		}
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
