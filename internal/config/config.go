package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogDir        string           `yaml:"log_dir"`
	ClickHouse    ClickHouseConfig `yaml:"clickhouse"`
	BatchSize     int              `yaml:"batch_size"`
	FlushInterval int              `yaml:"flush_interval_seconds"`
	// 采集后是否删除原始日志文件
	DeleteAfterCollect bool `yaml:"delete_after_collect"`
	// 删除前保留的最小时间（秒），防止删除正在写入的文件
	DeleteMinAge int `yaml:"delete_min_age_seconds"`
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
		DeleteMinAge:  300, // 默认 5 分钟
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
