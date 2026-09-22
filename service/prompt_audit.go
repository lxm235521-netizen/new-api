package service

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

// 定向请求审计（prompt audit）
//
// 默认关闭。仅当 PROMPT_AUDIT_ENABLED=true 且命中 PROMPT_AUDIT_USERS /
// PROMPT_AUDIT_MODELS 过滤条件时，才把请求/响应正文追加写入 JSONL 文件，
// 每行一条记录，未命中的请求不会产生任何 IO，也不会写入数据库日志。
//
// 环境变量：
//
//	PROMPT_AUDIT_ENABLED     true/false，默认 false
//	PROMPT_AUDIT_USERS       逗号分隔，用户名或用户 id；留空表示所有用户
//	PROMPT_AUDIT_MODELS      逗号分隔模型名，支持 * 通配；不带 * 时按前缀匹配
//	                         （推理模型常带 -high/-low 等后缀）；留空表示所有模型
//	PROMPT_AUDIT_FILE        输出文件路径；留空则写入 <LOG_DIR>/prompt-audit-YYYYMMDD.jsonl
//	PROMPT_AUDIT_MAX_BYTES   单条正文最大字节数，默认 65536，<=0 表示不截断
//	PROMPT_AUDIT_SKIP_OUTPUT true 表示只记录输入，不记录输出与流式分片
//
// 注意：文件内容包含用户原始输入，属于敏感数据，请限制文件权限、访问范围与留存时间。

const (
	// PromptAuditDirectionInput 客户端请求（已转换/注入系统提示词后的上游请求体）。
	PromptAuditDirectionInput = "input"
	// PromptAuditDirectionOutput 非流式响应正文。
	PromptAuditDirectionOutput = "output"
	// PromptAuditDirectionOutputChunk 流式响应分片（上游 SSE 原始行）。
	PromptAuditDirectionOutputChunk = "output_chunk"
)

const defaultPromptAuditMaxBytes = 64 * 1024

type promptAuditConfig struct {
	enabled    bool
	userIds    map[int]struct{}
	usernames  map[string]struct{}
	models     []string
	file       string
	maxBytes   int
	skipOutput bool
}

var (
	promptAuditOnce sync.Once
	promptAuditCfg  promptAuditConfig

	promptAuditFileMu   sync.Mutex
	promptAuditFile     *os.File
	promptAuditFilePath string
)

func loadPromptAuditConfig() promptAuditConfig {
	cfg := promptAuditConfig{
		enabled:    common.GetEnvOrDefaultBool("PROMPT_AUDIT_ENABLED", false),
		file:       strings.TrimSpace(os.Getenv("PROMPT_AUDIT_FILE")),
		maxBytes:   common.GetEnvOrDefault("PROMPT_AUDIT_MAX_BYTES", defaultPromptAuditMaxBytes),
		skipOutput: common.GetEnvOrDefaultBool("PROMPT_AUDIT_SKIP_OUTPUT", false),
		userIds:    make(map[int]struct{}),
		usernames:  make(map[string]struct{}),
	}
	for _, item := range strings.Split(os.Getenv("PROMPT_AUDIT_USERS"), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if userId, err := strconv.Atoi(item); err == nil {
			cfg.userIds[userId] = struct{}{}
			continue
		}
		cfg.usernames[strings.ToLower(item)] = struct{}{}
	}
	for _, item := range strings.Split(os.Getenv("PROMPT_AUDIT_MODELS"), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		cfg.models = append(cfg.models, strings.ToLower(item))
	}
	return cfg
}

func getPromptAuditConfig() promptAuditConfig {
	promptAuditOnce.Do(func() {
		promptAuditCfg = loadPromptAuditConfig()
	})
	return promptAuditCfg
}

// PromptAuditEnabled 供调用方在拼接大字符串前短路判断，避免无谓开销。
func PromptAuditEnabled() bool {
	return getPromptAuditConfig().enabled
}

func (cfg promptAuditConfig) matchUser(userId int, username string) bool {
	if len(cfg.userIds) == 0 && len(cfg.usernames) == 0 {
		return true
	}
	if _, ok := cfg.userIds[userId]; ok {
		return true
	}
	if username == "" {
		return false
	}
	_, ok := cfg.usernames[strings.ToLower(strings.TrimSpace(username))]
	return ok
}

func (cfg promptAuditConfig) matchModel(models ...string) bool {
	if len(cfg.models) == 0 {
		return true
	}
	for _, model := range models {
		model = strings.ToLower(strings.TrimSpace(model))
		if model == "" {
			continue
		}
		for _, pattern := range cfg.models {
			if pattern == model {
				return true
			}
			if strings.Contains(pattern, "*") {
				if matched, err := path.Match(pattern, model); err == nil && matched {
					return true
				}
				continue
			}
			if strings.HasPrefix(model, pattern) {
				return true
			}
		}
	}
	return false
}

// AuditPrompt 记录一次请求或响应正文。
// 未启用、未命中过滤条件或正文为空时直接返回；info 允许为 nil，
// 此时身份信息退化为从 gin.Context 中读取。
func AuditPrompt(c *gin.Context, info *relaycommon.RelayInfo, direction string, payload []byte) {
	cfg := getPromptAuditConfig()
	if !cfg.enabled || len(payload) == 0 {
		return
	}
	if cfg.skipOutput && direction != PromptAuditDirectionInput {
		return
	}

	var (
		userId        int
		username      string
		tokenName     string
		requestId     string
		group         string
		model         string
		upstreamModel string
		channelId     int
		isStream      bool
	)
	if info != nil {
		userId = info.UserId
		group = info.UsingGroup
		model = info.OriginModelName
		isStream = info.IsStream
		// ChannelMeta 是内嵌指针，未初始化时不能直接读其字段。
		if info.ChannelMeta != nil {
			channelId = info.ChannelMeta.ChannelId
			upstreamModel = info.ChannelMeta.UpstreamModelName
		}
	}
	if c != nil {
		if userId == 0 {
			userId = c.GetInt("id")
		}
		username = c.GetString("username")
		tokenName = c.GetString("token_name")
		requestId = c.GetString(common.RequestIdKey)
		if group == "" {
			group = c.GetString("group")
		}
		if model == "" {
			model = c.GetString("original_model")
		}
		if channelId == 0 {
			channelId = c.GetInt("channel_id")
		}
	}

	if !cfg.matchUser(userId, username) {
		return
	}
	if !cfg.matchModel(model, upstreamModel) {
		return
	}

	originalBytes := len(payload)
	truncated := false
	if cfg.maxBytes > 0 && originalBytes > cfg.maxBytes {
		payload = trimIncompleteTrailingRune(payload[:cfg.maxBytes])
		truncated = true
	}

	record := map[string]interface{}{
		"time":           time.Now().Format(time.RFC3339Nano),
		"direction":      direction,
		"request_id":     requestId,
		"user_id":        userId,
		"username":       username,
		"token_name":     tokenName,
		"channel_id":     channelId,
		"group":          group,
		"model":          model,
		"upstream_model": upstreamModel,
		"is_stream":      isStream,
		"payload_bytes":  originalBytes,
		"truncated":      truncated,
		"payload":        string(payload),
	}
	line, err := common.Marshal(record)
	if err != nil {
		common.SysLog("prompt audit marshal failed: " + err.Error())
		return
	}
	if err := appendPromptAuditLine(line); err != nil {
		common.SysLog("prompt audit write failed: " + err.Error())
	}
}

// AuditRequestBodyStorage 用于透传（passthrough）路径：此时上游请求体就是客户端原始
// 请求体，直接从这里读取。仅在审计启用时才读取 body，未启用的部署不产生额外内存拷贝。
func AuditRequestBodyStorage(c *gin.Context, info *relaycommon.RelayInfo, storage common.BodyStorage) {
	if storage == nil || !PromptAuditEnabled() {
		return
	}
	payload, err := storage.Bytes()
	if err != nil {
		common.SysLog("prompt audit read body failed: " + err.Error())
		return
	}
	AuditPrompt(c, info, PromptAuditDirectionInput, payload)
}

func appendPromptAuditLine(line []byte) error {
	promptAuditFileMu.Lock()
	defer promptAuditFileMu.Unlock()

	filePath := resolvePromptAuditFilePath()
	if promptAuditFile == nil || promptAuditFilePath != filePath {
		if promptAuditFile != nil {
			_ = promptAuditFile.Close()
			promptAuditFile = nil
		}
		if dir := filepath.Dir(filePath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return err
			}
		}
		fd, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		promptAuditFile = fd
		promptAuditFilePath = filePath
	}
	_, err := promptAuditFile.Write(append(line, '\n'))
	return err
}

func resolvePromptAuditFilePath() string {
	cfg := getPromptAuditConfig()
	if cfg.file != "" {
		return cfg.file
	}
	fileName := fmt.Sprintf("prompt-audit-%s.jsonl", time.Now().Format("20060102"))
	if common.LogDir != nil && *common.LogDir != "" {
		return filepath.Join(*common.LogDir, fileName)
	}
	return fileName
}

// trimIncompleteTrailingRune 去掉被截断的多字节字符尾部，避免写出非法 UTF-8。
func trimIncompleteTrailingRune(payload []byte) []byte {
	for len(payload) > 0 {
		r, size := utf8.DecodeLastRune(payload)
		if r != utf8.RuneError || size > 1 {
			break
		}
		payload = payload[:len(payload)-1]
	}
	return payload
}

// resetPromptAuditForTest 清空懒加载配置与文件句柄，仅供测试使用。
func resetPromptAuditForTest() {
	promptAuditFileMu.Lock()
	if promptAuditFile != nil {
		_ = promptAuditFile.Close()
		promptAuditFile = nil
	}
	promptAuditFilePath = ""
	promptAuditFileMu.Unlock()

	promptAuditOnce = sync.Once{}
	promptAuditCfg = promptAuditConfig{}
}
