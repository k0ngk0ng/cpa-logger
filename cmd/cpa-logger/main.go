package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/k0ngk0ng/cpa-logger/internal/collector"
	"github.com/k0ngk0ng/cpa-logger/internal/config"
	"github.com/k0ngk0ng/cpa-logger/internal/storage"
)

var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	configPath := flag.String("config", "/etc/cpa-logger/config.yaml", "Path to config file")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		log.Printf("cpa-logger version %s (commit: %s, built: %s)", version, commit, buildTime)
		os.Exit(0)
	}

	log.Printf("Starting cpa-logger %s...", version)

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Log directory: %s", cfg.LogDir)
	log.Printf("ClickHouse: %s:%d/%s", cfg.ClickHouse.Host, cfg.ClickHouse.Port, cfg.ClickHouse.Database)

	// 检查日志目录
	if _, err := os.Stat(cfg.LogDir); os.IsNotExist(err) {
		log.Fatalf("Log directory does not exist: %s", cfg.LogDir)
	}

	// 连接 ClickHouse
	store, err := storage.NewClickHouseStorage(&cfg.ClickHouse)
	if err != nil {
		log.Fatalf("Failed to connect to ClickHouse: %v", err)
	}
	log.Println("Connected to ClickHouse")

	// 创建采集器
	col, err := collector.New(cfg, store)
	if err != nil {
		log.Fatalf("Failed to create collector: %v", err)
	}

	// 启动采集器
	if err := col.Start(); err != nil {
		log.Fatalf("Failed to start collector: %v", err)
	}

	log.Println("Collector started successfully")

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	col.Stop()
	log.Println("Bye!")
}
