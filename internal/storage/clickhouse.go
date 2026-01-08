package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/k0ngk0ng/cpa-logger/internal/config"
	"github.com/k0ngk0ng/cpa-logger/internal/parser"
)

type ClickHouseStorage struct {
	conn     driver.Conn
	database string
}

func NewClickHouseStorage(cfg *config.ClickHouseConfig) (*ClickHouseStorage, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:     30 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	s := &ClickHouseStorage{
		conn:     conn,
		database: cfg.Database,
	}

	if err := s.createTables(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *ClickHouseStorage) createTables() error {
	ctx := context.Background()

	// 创建数据库
	if err := s.conn.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", s.database)); err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	// 主日志表
	mainLogTable := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.main_logs (
			timestamp DateTime64(3),
			request_id String,
			level LowCardinality(String),
			source String,
			message String,
			status_code UInt16,
			latency String,
			client_ip String,
			method LowCardinality(String),
			path String,
			log_file String,
			inserted_at DateTime64(3) DEFAULT now64(3)
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMMDD(timestamp)
		ORDER BY (timestamp, request_id)
		TTL toDateTime(timestamp) + INTERVAL 90 DAY
	`, s.database)
	if err := s.conn.Exec(ctx, mainLogTable); err != nil {
		return fmt.Errorf("failed to create main_logs table: %w", err)
	}

	// API 请求日志表
	apiLogTable := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.api_logs (
			log_type LowCardinality(String),
			request_id String,
			timestamp DateTime64(3),
			version String,
			url String,
			method LowCardinality(String),
			headers String,
			request_body String,
			response_status UInt16,
			response_headers String,
			response_body String,
			full_response String,
			upstream_requests String,
			log_file String,
			inserted_at DateTime64(3) DEFAULT now64(3)
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMMDD(timestamp)
		ORDER BY (timestamp, request_id)
		TTL toDateTime(timestamp) + INTERVAL 90 DAY
	`, s.database)
	if err := s.conn.Exec(ctx, apiLogTable); err != nil {
		return fmt.Errorf("failed to create api_logs table: %w", err)
	}

	// 事件批量日志表
	eventLogTable := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.event_logs (
			request_id String,
			timestamp DateTime64(3),
			event_type String,
			event_name String,
			session_id String,
			model String,
			user_type String,
			platform String,
			device_id String,
			event_data String,
			log_file String,
			inserted_at DateTime64(3) DEFAULT now64(3)
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMMDD(timestamp)
		ORDER BY (timestamp, session_id, event_name)
		TTL toDateTime(timestamp) + INTERVAL 90 DAY
	`, s.database)
	if err := s.conn.Exec(ctx, eventLogTable); err != nil {
		return fmt.Errorf("failed to create event_logs table: %w", err)
	}

	// 文件处理记录表（用于避免重复处理）
	fileTrackTable := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.processed_files (
			file_path String,
			file_size UInt64,
			file_mtime DateTime64(3),
			processed_at DateTime64(3) DEFAULT now64(3),
			record_count UInt32
		) ENGINE = ReplacingMergeTree(processed_at)
		ORDER BY file_path
	`, s.database)
	if err := s.conn.Exec(ctx, fileTrackTable); err != nil {
		return fmt.Errorf("failed to create processed_files table: %w", err)
	}

	return nil
}

// InsertMainLogs 批量插入主日志
func (s *ClickHouseStorage) InsertMainLogs(ctx context.Context, entries []parser.MainLogEntry, logFile string) error {
	if len(entries) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, fmt.Sprintf(`
		INSERT INTO %s.main_logs (
			timestamp, request_id, level, source, message,
			status_code, latency, client_ip, method, path, log_file
		) VALUES
	`, s.database))
	if err != nil {
		return err
	}

	for _, e := range entries {
		if err := batch.Append(
			e.Timestamp,
			e.RequestID,
			e.Level,
			e.Source,
			e.Message,
			uint16(e.StatusCode),
			e.Latency,
			e.ClientIP,
			e.Method,
			e.Path,
			logFile,
		); err != nil {
			return err
		}
	}

	return batch.Send()
}

// InsertAPILog 插入 API 日志
func (s *ClickHouseStorage) InsertAPILog(ctx context.Context, entry *parser.APILogEntry, logFile string) error {
	if entry == nil {
		return nil
	}

	headersJSON, _ := json.Marshal(entry.Headers)
	respHeadersJSON, _ := json.Marshal(entry.ResponseHeaders)
	upstreamJSON, _ := json.Marshal(entry.UpstreamRequests)

	return s.conn.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.api_logs (
			log_type, request_id, timestamp, version, url, method,
			headers, request_body, response_status, response_headers,
			response_body, full_response, upstream_requests, log_file
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.database),
		string(entry.LogType),
		entry.RequestID,
		entry.Timestamp,
		entry.Version,
		entry.URL,
		entry.Method,
		string(headersJSON),
		entry.RequestBody,
		uint16(entry.ResponseStatus),
		string(respHeadersJSON),
		entry.ResponseBody,
		entry.FullResponse,
		string(upstreamJSON),
		logFile,
	)
}

// InsertEventBatch 插入事件批量日志
func (s *ClickHouseStorage) InsertEventBatch(ctx context.Context, entry *parser.EventBatchEntry, logFile string) error {
	if entry == nil || len(entry.Events) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, fmt.Sprintf(`
		INSERT INTO %s.event_logs (
			request_id, timestamp, event_type, event_name, session_id,
			model, user_type, platform, device_id, event_data, log_file
		) VALUES
	`, s.database))
	if err != nil {
		return err
	}

	for _, evt := range entry.Events {
		eventType, _ := evt["event_type"].(string)

		eventData, ok := evt["event_data"].(map[string]interface{})
		if !ok {
			continue
		}

		eventName, _ := eventData["event_name"].(string)
		sessionID, _ := eventData["session_id"].(string)
		model, _ := eventData["model"].(string)
		userType, _ := eventData["user_type"].(string)
		deviceID, _ := eventData["device_id"].(string)

		var platform string
		if env, ok := eventData["env"].(map[string]interface{}); ok {
			platform, _ = env["platform"].(string)
		}

		// 解析时间戳
		var ts time.Time
		if tsStr, ok := eventData["client_timestamp"].(string); ok {
			ts, _ = time.Parse(time.RFC3339, tsStr)
		}
		if ts.IsZero() {
			ts = entry.Timestamp
		}

		eventDataJSON, _ := json.Marshal(eventData)

		if err := batch.Append(
			entry.RequestID,
			ts,
			eventType,
			eventName,
			sessionID,
			model,
			userType,
			platform,
			deviceID,
			string(eventDataJSON),
			logFile,
		); err != nil {
			return err
		}
	}

	return batch.Send()
}

// MarkFileProcessed 标记文件已处理
func (s *ClickHouseStorage) MarkFileProcessed(ctx context.Context, filePath string, fileSize int64, mtime time.Time, recordCount uint32) error {
	return s.conn.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.processed_files (file_path, file_size, file_mtime, record_count)
		VALUES (?, ?, ?, ?)
	`, s.database), filePath, uint64(fileSize), mtime, recordCount)
}

// IsFileProcessed 检查文件是否已处理
func (s *ClickHouseStorage) IsFileProcessed(ctx context.Context, filePath string, fileSize int64, mtime time.Time) (bool, error) {
	var count uint64
	err := s.conn.QueryRow(ctx, fmt.Sprintf(`
		SELECT count() FROM %s.processed_files
		WHERE file_path = ? AND file_size = ? AND file_mtime = ?
	`, s.database), filePath, uint64(fileSize), mtime).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *ClickHouseStorage) Close() error {
	return s.conn.Close()
}
