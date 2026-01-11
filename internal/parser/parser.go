package parser

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LogType 日志类型
type LogType string

const (
	LogTypeMain              LogType = "main"
	LogTypeV1Messages        LogType = "v1_messages"
	LogTypeV1CountTokens     LogType = "v1_count_tokens"
	LogTypeProviderMessages  LogType = "provider_messages"
	LogTypeProviderCountTokens LogType = "provider_count_tokens"
	LogTypeProviderResponses LogType = "provider_responses"
	LogTypeEventBatch        LogType = "event_batch"
)

// MainLogEntry main.log 日志条目
type MainLogEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	RequestID   string    `json:"request_id"`
	Level       string    `json:"level"`
	Source      string    `json:"source"`
	Message     string    `json:"message"`
	StatusCode  int       `json:"status_code,omitempty"`
	Latency     string    `json:"latency,omitempty"`
	ClientIP    string    `json:"client_ip,omitempty"`
	Method      string    `json:"method,omitempty"`
	Path        string    `json:"path,omitempty"`
}

// APILogEntry API 请求日志条目
type APILogEntry struct {
	LogType      LogType   `json:"log_type"`
	RequestID    string    `json:"request_id"`
	Timestamp    time.Time `json:"timestamp"`
	Version      string    `json:"version"`
	URL          string    `json:"url"`
	Method       string    `json:"method"`
	Headers      map[string]string `json:"headers"`
	RequestBody  string    `json:"request_body"`
	ResponseStatus int     `json:"response_status"`
	ResponseHeaders map[string]string `json:"response_headers"`
	ResponseBody string    `json:"response_body"`
	// 对于流式响应，拼接后的完整内容
	FullResponse string    `json:"full_response,omitempty"`
	// 上游 API 请求/响应（用于 provider 类型）
	UpstreamRequests []UpstreamCall `json:"upstream_requests,omitempty"`
}

// UpstreamCall 上游 API 调用
type UpstreamCall struct {
	Index       int       `json:"index"`
	Timestamp   time.Time `json:"timestamp"`
	URL         string    `json:"url"`
	Method      string    `json:"method"`
	Headers     map[string]string `json:"headers"`
	Body        string    `json:"body"`
	Status      int       `json:"status"`
	RespHeaders map[string]string `json:"resp_headers"`
	RespBody    string    `json:"resp_body"`
}

// EventBatchEntry 事件批量日志
type EventBatchEntry struct {
	RequestID   string    `json:"request_id"`
	Timestamp   time.Time `json:"timestamp"`
	Events      []map[string]interface{} `json:"events"`
}

// 正则表达式
var (
	// main.log 格式: [2026-01-08 09:29:48] [a3523f75] [info ] [main.go:413] message
	mainLogPattern = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\] \[([^\]]+)\] \[(\w+)\s*\] \[([^\]]+)\] (.*)$`)
	// HTTP 日志格式: 404 |          98ms |   58.246.36.130 | POST    "/path"
	httpLogPattern = regexp.MustCompile(`(\d{3}) \|\s*([^\|]+)\|\s*([^\|]+)\| (\w+)\s+"([^"]+)"`)
	// 文件名匹配: v1-messages-2026-01-08T103603-6dcb09d0.log
	apiLogFilePattern = regexp.MustCompile(`^(.+)-(\d{4}-\d{2}-\d{2}T\d{6})-([a-f0-9]{8})\.log$`)
	// main 日志文件名: main-2026-01-08T12-44-49.243.log
	mainLogFilePattern = regexp.MustCompile(`^main-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3})\.log$`)
)

// DetermineLogType 根据文件名判断日志类型
func DetermineLogType(filename string) LogType {
	base := filepath.Base(filename)

	if mainLogFilePattern.MatchString(base) || base == "main.log" {
		return LogTypeMain
	}

	// 按前缀匹配
	switch {
	case strings.HasPrefix(base, "api-provider-agy-api-event_logging-batch"):
		return LogTypeEventBatch
	case strings.HasPrefix(base, "api-provider-agy-v1-messages-count_tokens"):
		return LogTypeProviderCountTokens
	case strings.HasPrefix(base, "api-provider-agy-responses"):
		return LogTypeProviderResponses
	case strings.HasPrefix(base, "api-provider-agy"):
		return LogTypeProviderMessages
	case strings.HasPrefix(base, "v1-messages-count_tokens"):
		return LogTypeV1CountTokens
	case strings.HasPrefix(base, "v1-messages"):
		return LogTypeV1Messages
	}

	return LogTypeMain
}

// ExtractRequestIDFromFilename 从文件名提取 request_id
func ExtractRequestIDFromFilename(filename string) string {
	base := filepath.Base(filename)
	matches := apiLogFilePattern.FindStringSubmatch(base)
	if len(matches) >= 4 {
		return matches[3]
	}
	return ""
}

// ParseMainLog 解析 main.log
func ParseMainLog(filepath string) ([]MainLogEntry, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []MainLogEntry
	scanner := bufio.NewScanner(file)
	// 增大缓冲区以处理长行
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		entry, ok := parseMainLogLine(line)
		if ok {
			entries = append(entries, entry)
		}
	}

	return entries, scanner.Err()
}

func parseMainLogLine(line string) (MainLogEntry, bool) {
	matches := mainLogPattern.FindStringSubmatch(line)
	if len(matches) < 6 {
		return MainLogEntry{}, false
	}

	ts, _ := time.ParseInLocation("2006-01-02 15:04:05", matches[1], time.Local)
	entry := MainLogEntry{
		Timestamp: ts,
		RequestID: matches[2],
		Level:     strings.TrimSpace(matches[3]),
		Source:    matches[4],
		Message:   matches[5],
	}

	// 尝试解析 HTTP 日志
	httpMatches := httpLogPattern.FindStringSubmatch(matches[5])
	if len(httpMatches) >= 6 {
		entry.StatusCode, _ = strconv.Atoi(httpMatches[1])
		entry.Latency = strings.TrimSpace(httpMatches[2])
		entry.ClientIP = strings.TrimSpace(httpMatches[3])
		entry.Method = strings.TrimSpace(httpMatches[4])
		entry.Path = httpMatches[5]
	}

	return entry, true
}

// ParseAPILog 解析 API 日志
func ParseAPILog(filepath string, logType LogType) (*APILogEntry, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	content := string(data)
	entry := &APILogEntry{
		LogType:   logType,
		RequestID: ExtractRequestIDFromFilename(filepath),
		Headers:   make(map[string]string),
		ResponseHeaders: make(map[string]string),
	}

	// 分段解析
	sections := splitSections(content)

	for name, body := range sections {
		switch {
		case name == "REQUEST INFO":
			parseRequestInfo(body, entry)
		case name == "HEADERS":
			entry.Headers = parseHeaders(body)
		case name == "REQUEST BODY":
			entry.RequestBody = strings.TrimSpace(body)
		case name == "RESPONSE":
			parseResponse(body, entry)
		case strings.HasPrefix(name, "API REQUEST"):
			idx := extractIndex(name)
			upstream := parseUpstreamRequest(body, idx)
			entry.UpstreamRequests = append(entry.UpstreamRequests, upstream)
		case strings.HasPrefix(name, "API RESPONSE"):
			idx := extractIndex(name)
			if idx > 0 && idx <= len(entry.UpstreamRequests) {
				parseUpstreamResponse(body, &entry.UpstreamRequests[idx-1])
			}
		}
	}

	// 处理流式响应：拼接完整内容
	entry.FullResponse = extractFullStreamResponse(entry.ResponseBody)

	return entry, nil
}

// ParseEventBatchLog 解析事件批量日志
func ParseEventBatchLog(filepath string) (*EventBatchEntry, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	content := string(data)
	sections := splitSections(content)

	entry := &EventBatchEntry{
		RequestID: ExtractRequestIDFromFilename(filepath),
	}

	// 解析时间戳
	if info, ok := sections["REQUEST INFO"]; ok {
		for _, line := range strings.Split(info, "\n") {
			if strings.HasPrefix(line, "Timestamp:") {
				tsStr := strings.TrimSpace(strings.TrimPrefix(line, "Timestamp:"))
				entry.Timestamp, _ = time.Parse(time.RFC3339Nano, tsStr)
				break
			}
		}
	}

	// 解析事件
	if body, ok := sections["REQUEST BODY"]; ok {
		body = strings.TrimSpace(body)
		var eventData struct {
			Events []map[string]interface{} `json:"events"`
		}
		if json.Unmarshal([]byte(body), &eventData) == nil {
			entry.Events = eventData.Events
		}
	}

	return entry, nil
}

// splitSections 分割日志的各个部分
func splitSections(content string) map[string]string {
	sections := make(map[string]string)
	sectionPattern := regexp.MustCompile(`(?m)^=== (.+?) ===\s*$`)

	matches := sectionPattern.FindAllStringSubmatchIndex(content, -1)
	for i, match := range matches {
		name := content[match[2]:match[3]]
		start := match[1]
		var end int
		if i+1 < len(matches) {
			end = matches[i+1][0]
		} else {
			end = len(content)
		}
		sections[name] = strings.TrimSpace(content[start:end])
	}

	return sections
}

func parseRequestInfo(body string, entry *APILogEntry) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Version:"):
			entry.Version = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		case strings.HasPrefix(line, "URL:"):
			entry.URL = strings.TrimSpace(strings.TrimPrefix(line, "URL:"))
		case strings.HasPrefix(line, "Method:"):
			entry.Method = strings.TrimSpace(strings.TrimPrefix(line, "Method:"))
		case strings.HasPrefix(line, "Timestamp:"):
			tsStr := strings.TrimSpace(strings.TrimPrefix(line, "Timestamp:"))
			entry.Timestamp, _ = time.Parse(time.RFC3339Nano, tsStr)
		}
	}
}

func parseHeaders(body string) map[string]string {
	headers := make(map[string]string)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			headers[key] = value
		}
	}
	return headers
}

func parseResponse(body string, entry *APILogEntry) {
	lines := strings.Split(body, "\n")
	headerDone := false
	var bodyLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			headerDone = true
			continue
		}
		if !headerDone {
			if strings.HasPrefix(line, "Status:") {
				statusStr := strings.TrimSpace(strings.TrimPrefix(line, "Status:"))
				entry.ResponseStatus, _ = strconv.Atoi(statusStr)
			} else if idx := strings.Index(line, ":"); idx > 0 {
				key := strings.TrimSpace(line[:idx])
				value := strings.TrimSpace(line[idx+1:])
				entry.ResponseHeaders[key] = value
			}
		} else {
			bodyLines = append(bodyLines, line)
		}
	}
	entry.ResponseBody = strings.Join(bodyLines, "\n")
}

func extractIndex(name string) int {
	re := regexp.MustCompile(`(\d+)`)
	if m := re.FindString(name); m != "" {
		idx, _ := strconv.Atoi(m)
		return idx
	}
	return 1
}

func parseUpstreamRequest(body string, idx int) UpstreamCall {
	call := UpstreamCall{
		Index:   idx,
		Headers: make(map[string]string),
	}

	lines := strings.Split(body, "\n")
	inHeaders := false
	inBody := false
	var bodyLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Timestamp:"):
			tsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "Timestamp:"))
			call.Timestamp, _ = time.Parse(time.RFC3339Nano, tsStr)
		case strings.HasPrefix(trimmed, "Upstream URL:"):
			call.URL = strings.TrimSpace(strings.TrimPrefix(trimmed, "Upstream URL:"))
		case strings.HasPrefix(trimmed, "HTTP Method:"):
			call.Method = strings.TrimSpace(strings.TrimPrefix(trimmed, "HTTP Method:"))
		case trimmed == "Headers:":
			inHeaders = true
			inBody = false
		case trimmed == "Body:":
			inHeaders = false
			inBody = true
		case inHeaders:
			if colonIdx := strings.Index(trimmed, ":"); colonIdx > 0 {
				key := strings.TrimSpace(trimmed[:colonIdx])
				value := strings.TrimSpace(trimmed[colonIdx+1:])
				call.Headers[key] = value
			}
		case inBody:
			bodyLines = append(bodyLines, line)
		}
	}
	call.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	return call
}

func parseUpstreamResponse(body string, call *UpstreamCall) {
	call.RespHeaders = make(map[string]string)

	lines := strings.Split(body, "\n")
	inHeaders := false
	inBody := false
	var bodyLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Status:"):
			statusStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "Status:"))
			call.Status, _ = strconv.Atoi(statusStr)
		case trimmed == "Headers:":
			inHeaders = true
			inBody = false
		case trimmed == "Body:":
			inHeaders = false
			inBody = true
		case inHeaders:
			if idx := strings.Index(trimmed, ":"); idx > 0 {
				key := strings.TrimSpace(trimmed[:idx])
				value := strings.TrimSpace(trimmed[idx+1:])
				call.RespHeaders[key] = value
			}
		case inBody:
			bodyLines = append(bodyLines, line)
		}
	}
	call.RespBody = strings.TrimSpace(strings.Join(bodyLines, "\n"))
}

// extractFullStreamResponse 提取流式响应中的完整文本内容
func extractFullStreamResponse(body string) string {
	// SSE 格式: data: {...}
	var fullContent strings.Builder
	lines := strings.Split(body, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data:")
		dataStr = strings.TrimSpace(dataStr)

		if dataStr == "[DONE]" {
			continue
		}

		// 尝试解析 JSON
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
			continue
		}

		// Claude 格式: delta.text 或 content_block_delta
		if delta, ok := data["delta"].(map[string]interface{}); ok {
			if text, ok := delta["text"].(string); ok {
				fullContent.WriteString(text)
			}
		}
		// OpenAI 格式: choices[0].delta.content
		if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if content, ok := delta["content"].(string); ok {
						fullContent.WriteString(content)
					}
				}
			}
		}
	}

	return fullContent.String()
}
