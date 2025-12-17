package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Broker BrokerConfig `yaml:"broker"`
	Log    LogConfig    `yaml:"log"`
}

type BrokerConfig struct {
	Address      string       `yaml:"address"`
	WSAddress    string       `yaml:"ws_address"`
	HTTPAddress  string       `yaml:"http_address"`
	KeepAlive    int          `yaml:"keep_alive"`
	MaxMessageSize int        `yaml:"max_message_size"`
	MaxClients   int          `yaml:"max_clients"`
	DataPath     string       `yaml:"data_path"`
	Cluster      ClusterConfig `yaml:"cluster"`
}

type ClusterConfig struct {
	Enabled   bool     `yaml:"enabled"`
	NodeID    string   `yaml:"node_id"`
	Nodes     []string `yaml:"nodes"`
	ClusterPort int    `yaml:"cluster_port"`
}

type LogConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	if cfg.Broker.Address == "" {
		cfg.Broker.Address = ":1883"
	}
	if cfg.Broker.WSAddress == "" {
		cfg.Broker.WSAddress = ":8083"
	}
	if cfg.Broker.HTTPAddress == "" {
		cfg.Broker.HTTPAddress = ":8080"
	}
	if cfg.Broker.KeepAlive == 0 {
		cfg.Broker.KeepAlive = 300
	}
	if cfg.Broker.MaxMessageSize == 0 {
		cfg.Broker.MaxMessageSize = 1048576
	}
	if cfg.Broker.MaxClients == 0 {
		cfg.Broker.MaxClients = 10000
	}
	if cfg.Broker.DataPath == "" {
		cfg.Broker.DataPath = "./data"
	}

	return &cfg, nil
}

