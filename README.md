# CPA Logger

CLIProxyAPI 日志采集器，将日志实时导入 ClickHouse 进行存储和分析。

## 功能特性

- 实时监控日志目录，自动处理新增日志文件
- 支持 5 种日志类型的解析：
  - `main` - 主应用日志（Gin HTTP 日志 + 应用日志）
  - `v1_messages` - Claude Messages API 请求/响应
  - `v1_count_tokens` - Token 计数 API
  - `provider_messages` - 上游 Provider API 日志
  - `provider_count_tokens` - 上游 Provider Token 计数
  - `event_batch` - 客户端遥测事件
- 自动提取流式响应的完整内容（`full_response` 字段）
- 文件去重处理，避免重复导入
- 使用 request_id 关联同一请求的多个日志
- 支持按日志类型单独配置采集和删除策略
- 采集后可选自动删除原始日志文件

## ClickHouse 表结构

### main_logs - 主日志表
```sql
-- 查询示例
SELECT timestamp, request_id, level, message
FROM cpa_logs.main_logs
WHERE timestamp > now() - INTERVAL 1 HOUR
ORDER BY timestamp DESC
LIMIT 100;
```

### api_logs - API 请求日志表
```sql
-- 查询请求详情
SELECT request_id, url, method, response_status, full_response
FROM cpa_logs.api_logs
WHERE request_id = 'a1b2c3d4';

-- 查询流式响应的完整内容
SELECT request_id, full_response
FROM cpa_logs.api_logs
WHERE full_response != ''
ORDER BY timestamp DESC
LIMIT 10;
```

### event_logs - 事件日志表
```sql
-- 按 session 查询事件
SELECT timestamp, event_name, model, event_data
FROM cpa_logs.event_logs
WHERE session_id = 'xxx'
ORDER BY timestamp;
```

## 安装

### 从 Release 安装

```bash
# 下载最新版本
wget https://github.com/k0ngk0ng/cpa-logger/releases/latest/download/cpa-logger-v0.1.0-linux-amd64.tar.gz

# 解压
tar -xzf cpa-logger-*.tar.gz

# 安装
chmod +x install.sh
sudo ./install.sh

# 编辑配置
sudo nano /etc/cpa-logger/config.yaml

# 启动服务
sudo systemctl start cpa-logger
sudo systemctl enable cpa-logger
```

### 从源码编译

```bash
git clone https://github.com/k0ngk0ng/cpa-logger.git
cd cpa-logger
make build
```

## 配置

配置文件位于 `/etc/cpa-logger/config.yaml`：

```yaml
# 日志目录 - CLIProxyAPI 生成日志的目录
log_dir: /var/log/cliproxyapi

# 批量处理设置
batch_size: 1000
flush_interval_seconds: 5

# 采集后删除设置（全局默认）
delete_after_collect: false
delete_min_age_seconds: 300  # 删除前文件必须存在的最小时间

# 各类型日志的采集配置
log_types:
  main:
    enabled: true
  v1_messages:
    enabled: true
  v1_count_tokens:
    enabled: true
  provider_messages:
    enabled: true
  provider_count_tokens:
    enabled: true
  event_batch:
    enabled: false  # 禁用事件批量日志采集
    # delete_after_collect: true  # 可单独覆盖全局删除策略

# ClickHouse 配置
clickhouse:
  host: localhost
  port: 9000
  database: cpa_logs
  username: default
  password: ""
```

### 配置说明

| 配置项 | 说明 | 默认值 |
|-------|------|-------|
| `log_dir` | CLIProxyAPI 日志目录 | - |
| `batch_size` | 批量插入条数 | 1000 |
| `flush_interval_seconds` | 刷新间隔 | 5 |
| `delete_after_collect` | 采集后删除原始日志 | false |
| `delete_min_age_seconds` | 删除前文件最小存在时间 | 300 |
| `log_types.<type>.enabled` | 是否采集该类型日志 | true |
| `log_types.<type>.delete_after_collect` | 覆盖全局删除策略 | - |

## 运行

### 作为 systemd 服务

```bash
# 启动
sudo systemctl start cpa-logger

# 停止
sudo systemctl stop cpa-logger

# 查看状态
sudo systemctl status cpa-logger

# 查看日志
sudo journalctl -u cpa-logger -f
```

### 手动运行

```bash
./cpa-logger -config /path/to/config.yaml
```

## 日志格式说明

### main 日志格式
```
[2026-01-08 09:29:48] [a3523f75] [info ] [main.go:413] Message...
[时间戳] [request_id] [级别] [源码位置] 消息内容
```

### API 日志格式
```
=== REQUEST INFO ===
Version: 6.6.88
URL: /v1/messages
Method: POST
Timestamp: 2026-01-08T10:36:03+08:00

=== HEADERS ===
X-Request-Id: 90e609dd...
...

=== REQUEST BODY ===
{...}

=== RESPONSE ===
Status: 200
Content-Type: application/json

{...}
```

## 许可证

MIT
