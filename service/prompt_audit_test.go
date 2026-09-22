package service

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	promptAuditSetting "github.com/QuantumNous/new-api/setting/prompt_audit_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// withPromptAuditSetting 设置后台配置项并在测试结束后还原；
// 同时把相关环境变量清空，保证用例不受外部环境影响。
func withPromptAuditSetting(t *testing.T, mutate func(s *promptAuditSetting.PromptAuditSetting)) {
	t.Helper()
	for _, key := range []string{
		"PROMPT_AUDIT_ENABLED", "PROMPT_AUDIT_USERS", "PROMPT_AUDIT_MODELS",
		"PROMPT_AUDIT_MAX_BYTES", "PROMPT_AUDIT_SKIP_OUTPUT", "PROMPT_AUDIT_FILE",
	} {
		t.Setenv(key, "")
	}

	setting := promptAuditSetting.GetPromptAuditSetting()
	original := *setting
	if mutate != nil {
		mutate(setting)
	}
	resetPromptAuditForTest()
	t.Cleanup(func() {
		*setting = original
		resetPromptAuditForTest()
	})
}

func newPromptAuditContext(t *testing.T, userId int, username string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("id", userId)
	c.Set("username", username)
	c.Set("token_name", "prod")
	c.Set(common.RequestIdKey, "req-test-1")
	return c
}

func readPromptAuditLines(t *testing.T, filePath string) []string {
	t.Helper()
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func TestPromptAuditDisabledWritesNothing(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	withPromptAuditSetting(t, func(s *promptAuditSetting.PromptAuditSetting) {
		s.Enabled = false
	})

	c := newPromptAuditContext(t, 7, "alice")
	AuditPrompt(c, &relaycommon.RelayInfo{UserId: 7, OriginModelName: "gemini-3.1-pro"}, PromptAuditDirectionInput, []byte(`{"messages":[]}`))

	require.Empty(t, readPromptAuditLines(t, filePath))
}

func TestPromptAuditMatchesUserAndModelPrefix(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	withPromptAuditSetting(t, func(s *promptAuditSetting.PromptAuditSetting) {
		s.Enabled = true
		s.Users = "alice,42"
		s.Models = "gemini-3.1-pro"
	})
	t.Setenv("PROMPT_AUDIT_FILE", filePath)

	matched := newPromptAuditContext(t, 7, "alice")
	matchedInfo := &relaycommon.RelayInfo{
		UserId:          7,
		OriginModelName: "gemini-3.1-pro",
		UsingGroup:      "default",
		IsStream:        true,
	}
	matchedInfo.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelId:         3,
		UpstreamModelName: "gemini-3.1-pro-high",
	}
	AuditPrompt(matched, matchedInfo, PromptAuditDirectionInput, []byte(`{"messages":[{"role":"user","content":"hi"}]}`))

	// 命中用户但模型不匹配 -> 不记录
	otherModel := newPromptAuditContext(t, 7, "alice")
	AuditPrompt(otherModel, &relaycommon.RelayInfo{UserId: 7, OriginModelName: "gpt-4o"}, PromptAuditDirectionInput, []byte(`{"messages":[]}`))

	// 模型匹配但用户不匹配 -> 不记录
	otherUser := newPromptAuditContext(t, 8, "bob")
	AuditPrompt(otherUser, &relaycommon.RelayInfo{UserId: 8, OriginModelName: "gemini-3.1-pro"}, PromptAuditDirectionInput, []byte(`{"messages":[]}`))

	// 用用户 id 命中 -> 记录
	byId := newPromptAuditContext(t, 42, "whoever")
	AuditPrompt(byId, &relaycommon.RelayInfo{UserId: 42, OriginModelName: "gemini-3.1-pro-preview"}, PromptAuditDirectionInput, []byte(`{"messages":[]}`))

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], `"username":"alice"`)
	require.Contains(t, lines[0], `"upstream_model":"gemini-3.1-pro-high"`)
	require.Contains(t, lines[0], `"direction":"input"`)
	require.Contains(t, lines[0], `"request_id":"req-test-1"`)
	require.Contains(t, lines[0], `hi`)
	require.Contains(t, lines[1], `"username":"whoever"`)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestPromptAuditTruncatesMultibytePayload(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	withPromptAuditSetting(t, func(s *promptAuditSetting.PromptAuditSetting) {
		s.Enabled = true
		s.MaxBytes = 5
	})
	t.Setenv("PROMPT_AUDIT_FILE", filePath)

	c := newPromptAuditContext(t, 7, "alice")
	AuditPrompt(c, nil, PromptAuditDirectionInput, []byte("中文内容abc"))

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 1)
	// "中文" 共 6 字节，截到 5 字节会把第三个字截断，应回退到 3 字节
	require.Contains(t, lines[0], `"payload":"中"`)
	require.Contains(t, lines[0], `"truncated":true`)
	require.Contains(t, lines[0], `"payload_bytes":15`)
}

func TestPromptAuditNilChannelMetaDoesNotPanic(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	withPromptAuditSetting(t, func(s *promptAuditSetting.PromptAuditSetting) {
		s.Enabled = true
		s.Models = "gemini-3.1-pro"
	})
	t.Setenv("PROMPT_AUDIT_FILE", filePath)

	c := newPromptAuditContext(t, 7, "alice")
	c.Set("channel_id", 9)
	AuditPrompt(c, &relaycommon.RelayInfo{UserId: 7, OriginModelName: "gemini-3.1-pro"}, PromptAuditDirectionInput, []byte("in"))

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], `"channel_id":9`)
}

func TestPromptAuditSkipOutput(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	withPromptAuditSetting(t, func(s *promptAuditSetting.PromptAuditSetting) {
		s.Enabled = true
		s.SkipOutput = true
	})
	t.Setenv("PROMPT_AUDIT_FILE", filePath)

	c := newPromptAuditContext(t, 7, "alice")
	AuditPrompt(c, nil, PromptAuditDirectionInput, []byte("in"))
	AuditPrompt(c, nil, PromptAuditDirectionOutputChunk, []byte("out"))

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], `"direction":"input"`)
}

// 后台开启开关后无需重启即可生效（配置读取不再做一次性缓存）。
func TestPromptAuditSettingHotReload(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	withPromptAuditSetting(t, nil)
	t.Setenv("PROMPT_AUDIT_FILE", filePath)

	setting := promptAuditSetting.GetPromptAuditSetting()
	c := newPromptAuditContext(t, 7, "alice")

	AuditPrompt(c, nil, PromptAuditDirectionInput, []byte("before-enable"))
	require.Empty(t, readPromptAuditLines(t, filePath))

	setting.Enabled = true
	setting.Users = "alice"
	AuditPrompt(c, nil, PromptAuditDirectionInput, []byte("after-enable"))

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], "after-enable")

	// 关掉后立刻停止记录
	setting.Enabled = false
	AuditPrompt(c, nil, PromptAuditDirectionInput, []byte("after-disable"))
	require.Len(t, readPromptAuditLines(t, filePath), 1)
}

// 环境变量显式设置时覆盖后台配置（应急开关）。
func TestPromptAuditEnvOverridesSetting(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	withPromptAuditSetting(t, func(s *promptAuditSetting.PromptAuditSetting) {
		s.Enabled = true
		s.Users = "alice"
	})
	t.Setenv("PROMPT_AUDIT_FILE", filePath)

	// 后台开着，但环境变量强制关闭
	t.Setenv("PROMPT_AUDIT_ENABLED", "false")
	c := newPromptAuditContext(t, 7, "alice")
	AuditPrompt(c, nil, PromptAuditDirectionInput, []byte("env-disabled"))
	require.Empty(t, readPromptAuditLines(t, filePath))

	// 环境变量把监控对象改成 bob
	t.Setenv("PROMPT_AUDIT_ENABLED", "true")
	t.Setenv("PROMPT_AUDIT_USERS", "bob")
	AuditPrompt(c, nil, PromptAuditDirectionInput, []byte("alice-should-be-skipped"))
	AuditPrompt(newPromptAuditContext(t, 9, "bob"), nil, PromptAuditDirectionInput, []byte("bob-recorded"))

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], "bob-recorded")
}

// 文件路径未指定时落在 LOG_DIR 下，按天分文件。
func TestPromptAuditDefaultFilePathUsesLogDir(t *testing.T) {
	logDir := t.TempDir()
	original := *common.LogDir
	*common.LogDir = logDir
	t.Cleanup(func() { *common.LogDir = original })

	withPromptAuditSetting(t, func(s *promptAuditSetting.PromptAuditSetting) {
		s.Enabled = true
	})
	resetPromptAuditForTest()

	c := newPromptAuditContext(t, 7, "alice")
	AuditPrompt(c, nil, PromptAuditDirectionInput, []byte("in"))

	matches, err := filepath.Glob(filepath.Join(logDir, "prompt-audit-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
}

// 透传路径：从请求体存储中读取原始 body。
func TestPromptAuditRequestBodyStorage(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	withPromptAuditSetting(t, func(s *promptAuditSetting.PromptAuditSetting) {
		s.Enabled = true
	})
	t.Setenv("PROMPT_AUDIT_FILE", filePath)

	storage := newTestBodyStorage(t, []byte(`{"messages":[{"role":"user","content":"passthrough-body"}]}`))
	c := newPromptAuditContext(t, 7, "alice")
	AuditRequestBodyStorage(c, nil, storage)

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], "passthrough-body")

	// 未启用时不应读取/写入
	resetPromptAuditForTest()
	promptAuditSetting.GetPromptAuditSetting().Enabled = false
	AuditRequestBodyStorage(c, nil, storage)
	require.Len(t, readPromptAuditLines(t, filePath), 1)
}

func newTestBodyStorage(t *testing.T, body []byte) common.BodyStorage {
	t.Helper()
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })
	return storage
}
