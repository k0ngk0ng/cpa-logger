package collector

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/k0ngk0ng/cpa-logger/internal/config"
	"github.com/k0ngk0ng/cpa-logger/internal/parser"
	"github.com/k0ngk0ng/cpa-logger/internal/storage"
)

type Collector struct {
	cfg     *config.Config
	storage *storage.ClickHouseStorage
	watcher *fsnotify.Watcher
	done    chan struct{}
	wg      sync.WaitGroup
}

func New(cfg *config.Config, store *storage.ClickHouseStorage) (*Collector, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Collector{
		cfg:     cfg,
		storage: store,
		watcher: watcher,
		done:    make(chan struct{}),
	}, nil
}

func (c *Collector) Start() error {
	// 首先处理现有文件
	log.Println("Processing existing log files...")
	if err := c.processExistingFiles(); err != nil {
		log.Printf("Warning: error processing existing files: %v", err)
	}

	// 添加目录监控
	if err := c.watcher.Add(c.cfg.LogDir); err != nil {
		return err
	}
	log.Printf("Watching directory: %s", c.cfg.LogDir)

	// 启动文件监控
	c.wg.Add(1)
	go c.watchLoop()

	return nil
}

func (c *Collector) Stop() {
	close(c.done)
	c.watcher.Close()
	c.wg.Wait()
	c.storage.Close()
	log.Println("Collector stopped")
}

func (c *Collector) processExistingFiles() error {
	entries, err := os.ReadDir(c.cfg.LogDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		filePath := filepath.Join(c.cfg.LogDir, entry.Name())
		c.processFile(filePath)
	}

	return nil
}

func (c *Collector) watchLoop() {
	defer c.wg.Done()

	// 防止重复处理的去重 map
	recentlyProcessed := make(map[string]time.Time)
	var mu sync.Mutex

	// 定期清理去重 map
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return

		case event, ok := <-c.watcher.Events:
			if !ok {
				return
			}

			// 只处理创建和写入事件
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}

			// 只处理 .log 文件
			if !strings.HasSuffix(event.Name, ".log") {
				continue
			}

			// 去重：避免短时间内重复处理同一文件
			mu.Lock()
			lastProcessed, exists := recentlyProcessed[event.Name]
			if exists && time.Since(lastProcessed) < 2*time.Second {
				mu.Unlock()
				continue
			}
			recentlyProcessed[event.Name] = time.Now()
			mu.Unlock()

			// 延迟处理，确保文件写入完成
			time.AfterFunc(500*time.Millisecond, func() {
				c.processFile(event.Name)
			})

		case err, ok := <-c.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)

		case <-ticker.C:
			// 清理超过 10 分钟的去重记录
			mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for k, v := range recentlyProcessed {
				if v.Before(cutoff) {
					delete(recentlyProcessed, k)
				}
			}
			mu.Unlock()
		}
	}
}

func (c *Collector) processFile(filePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 获取文件信息
	info, err := os.Stat(filePath)
	if err != nil {
		log.Printf("Error getting file info %s: %v", filePath, err)
		return
	}

	// 检查是否已处理
	processed, err := c.storage.IsFileProcessed(ctx, filePath, info.Size(), info.ModTime())
	if err != nil {
		log.Printf("Error checking file status %s: %v", filePath, err)
		return
	}
	if processed {
		return
	}

	logType := parser.DetermineLogType(filePath)
	logTypeStr := string(logType)
	var recordCount uint32

	// 检查该日志类型是否启用采集
	typeConfig := c.cfg.GetLogTypeConfig(logTypeStr)
	if !typeConfig.Enabled {
		return
	}

	log.Printf("Processing file: %s (type: %s)", filepath.Base(filePath), logType)

	switch logType {
	case parser.LogTypeMain:
		entries, err := parser.ParseMainLog(filePath)
		if err != nil {
			log.Printf("Error parsing main log %s: %v", filePath, err)
			return
		}

		// 批量插入
		batchSize := c.cfg.BatchSize
		for i := 0; i < len(entries); i += batchSize {
			end := i + batchSize
			if end > len(entries) {
				end = len(entries)
			}

			if err := c.storage.InsertMainLogs(ctx, entries[i:end], filePath); err != nil {
				log.Printf("Error inserting main logs: %v", err)
				return
			}
		}
		recordCount = uint32(len(entries))

	case parser.LogTypeV1Messages, parser.LogTypeV1CountTokens,
		parser.LogTypeProviderMessages, parser.LogTypeProviderCountTokens:
		entry, err := parser.ParseAPILog(filePath, logType)
		if err != nil {
			log.Printf("Error parsing API log %s: %v", filePath, err)
			return
		}

		if err := c.storage.InsertAPILog(ctx, entry, filePath); err != nil {
			log.Printf("Error inserting API log: %v", err)
			return
		}
		recordCount = 1

	case parser.LogTypeEventBatch:
		entry, err := parser.ParseEventBatchLog(filePath)
		if err != nil {
			log.Printf("Error parsing event batch log %s: %v", filePath, err)
			return
		}

		if err := c.storage.InsertEventBatch(ctx, entry, filePath); err != nil {
			log.Printf("Error inserting event batch: %v", err)
			return
		}
		recordCount = uint32(len(entry.Events))
	}

	// 标记文件已处理
	if err := c.storage.MarkFileProcessed(ctx, filePath, info.Size(), info.ModTime(), recordCount); err != nil {
		log.Printf("Error marking file as processed: %v", err)
	} else {
		log.Printf("Processed %s: %d records", filepath.Base(filePath), recordCount)

		// 根据配置决定是否删除文件（支持按类型单独配置）
		if c.cfg.ShouldDeleteAfterCollect(logTypeStr) {
			c.tryDeleteFile(filePath, info)
		}
	}
}

// tryDeleteFile 尝试删除已处理的日志文件
func (c *Collector) tryDeleteFile(filePath string, info os.FileInfo) {
	// 检查文件年龄，避免删除正在写入的文件
	minAge := time.Duration(c.cfg.DeleteMinAge) * time.Second
	if time.Since(info.ModTime()) < minAge {
		log.Printf("Skipping delete (file too new): %s", filepath.Base(filePath))
		return
	}

	// 不删除 main.log（当前正在写入的主日志）
	if filepath.Base(filePath) == "main.log" {
		return
	}

	if err := os.Remove(filePath); err != nil {
		log.Printf("Error deleting file %s: %v", filepath.Base(filePath), err)
	} else {
		log.Printf("Deleted processed file: %s", filepath.Base(filePath))
	}
}
