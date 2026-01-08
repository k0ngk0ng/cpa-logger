package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogDir     string          `yaml:"log_dir"`
	ClickHouse ClickHouseConfig `yaml:"clickhouse"`
	BatchSize  int             `yaml:"batch_size"`
	FlushInterval int          `yaml:"flush_interval_seconds"`
}

type ClickHouseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		BatchSize:     1000,
		FlushInterval: 5,
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.ClickHouse.Port == 0 {
		cfg.ClickHouse.Port = 9000
	}
	if cfg.ClickHouse.Database == "" {
		cfg.ClickHouse.Database = "cpa_logs"
	}

	return cfg, nil
}
