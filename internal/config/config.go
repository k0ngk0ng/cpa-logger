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
	// 各类型日志的采集配置
	LogTypes LogTypesConfig `yaml:"log_types"`
}

// LogTypesConfig 各类型日志的采集配置
type LogTypesConfig struct {
	Main                 LogTypeConfig `yaml:"main"`
	V1Messages           LogTypeConfig `yaml:"v1_messages"`
	V1CountTokens        LogTypeConfig `yaml:"v1_count_tokens"`
	ProviderMessages     LogTypeConfig `yaml:"provider_messages"`
	ProviderCountTokens  LogTypeConfig `yaml:"provider_count_tokens"`
	ProviderResponses    LogTypeConfig `yaml:"provider_responses"`
	EventBatch           LogTypeConfig `yaml:"event_batch"`
}

// LogTypeConfig 单个日志类型配置
type LogTypeConfig struct {
	Enabled            bool `yaml:"enabled"`
	DeleteAfterCollect *bool `yaml:"delete_after_collect,omitempty"` // 覆盖全局配置
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
		LogTypes: LogTypesConfig{
			Main:                LogTypeConfig{Enabled: true},
			V1Messages:          LogTypeConfig{Enabled: true},
			V1CountTokens:       LogTypeConfig{Enabled: true},
			ProviderMessages:    LogTypeConfig{Enabled: true},
			ProviderCountTokens: LogTypeConfig{Enabled: true},
			ProviderResponses:   LogTypeConfig{Enabled: true},
			EventBatch:          LogTypeConfig{Enabled: true},
		},
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

// GetLogTypeConfig 获取指定日志类型的配置
func (c *Config) GetLogTypeConfig(logType string) LogTypeConfig {
	switch logType {
	case "main":
		return c.LogTypes.Main
	case "v1_messages":
		return c.LogTypes.V1Messages
	case "v1_count_tokens":
		return c.LogTypes.V1CountTokens
	case "provider_messages":
		return c.LogTypes.ProviderMessages
	case "provider_count_tokens":
		return c.LogTypes.ProviderCountTokens
	case "provider_responses":
		return c.LogTypes.ProviderResponses
	case "event_batch":
		return c.LogTypes.EventBatch
	default:
		return LogTypeConfig{Enabled: true}
	}
}

// ShouldDeleteAfterCollect 判断指定日志类型是否应该在采集后删除
func (c *Config) ShouldDeleteAfterCollect(logType string) bool {
	typeConfig := c.GetLogTypeConfig(logType)
	// 如果单独配置了，使用单独配置
	if typeConfig.DeleteAfterCollect != nil {
		return *typeConfig.DeleteAfterCollect
	}
	// 否则使用全局配置
	return c.DeleteAfterCollect
}
