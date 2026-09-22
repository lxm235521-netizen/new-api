package service

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
	resetPromptAuditForTest()
	t.Cleanup(resetPromptAuditForTest)
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("PROMPT_AUDIT_ENABLED", "false")
	t.Setenv("PROMPT_AUDIT_FILE", filePath)

	c := newPromptAuditContext(t, 7, "alice")
	AuditPrompt(c, &relaycommon.RelayInfo{UserId: 7, OriginModelName: "gemini-3.1-pro"}, PromptAuditDirectionInput, []byte(`{"messages":[]}`))

	require.Empty(t, readPromptAuditLines(t, filePath))
}

func TestPromptAuditMatchesUserAndModelPrefix(t *testing.T) {
	resetPromptAuditForTest()
	t.Cleanup(resetPromptAuditForTest)
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("PROMPT_AUDIT_ENABLED", "true")
	t.Setenv("PROMPT_AUDIT_FILE", filePath)
	t.Setenv("PROMPT_AUDIT_USERS", "alice,42")
	t.Setenv("PROMPT_AUDIT_MODELS", "gemini-3.1-pro")

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

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], `"username":"alice"`)
	require.Contains(t, lines[0], `"upstream_model":"gemini-3.1-pro-high"`)
	require.Contains(t, lines[0], `"direction":"input"`)
	require.Contains(t, lines[0], `"request_id":"req-test-1"`)
	require.Contains(t, lines[0], `hi`)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestPromptAuditTruncatesMultibytePayload(t *testing.T) {
	resetPromptAuditForTest()
	t.Cleanup(resetPromptAuditForTest)
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("PROMPT_AUDIT_ENABLED", "true")
	t.Setenv("PROMPT_AUDIT_FILE", filePath)
	t.Setenv("PROMPT_AUDIT_MAX_BYTES", "5")

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
	resetPromptAuditForTest()
	t.Cleanup(resetPromptAuditForTest)
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("PROMPT_AUDIT_ENABLED", "true")
	t.Setenv("PROMPT_AUDIT_FILE", filePath)
	t.Setenv("PROMPT_AUDIT_MODELS", "gemini-3.1-pro")

	c := newPromptAuditContext(t, 7, "alice")
	c.Set("channel_id", 9)
	AuditPrompt(c, &relaycommon.RelayInfo{UserId: 7, OriginModelName: "gemini-3.1-pro"}, PromptAuditDirectionInput, []byte("in"))

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], `"channel_id":9`)
}

func TestPromptAuditSkipOutput(t *testing.T) {
	resetPromptAuditForTest()
	t.Cleanup(resetPromptAuditForTest)
	filePath := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("PROMPT_AUDIT_ENABLED", "true")
	t.Setenv("PROMPT_AUDIT_FILE", filePath)
	t.Setenv("PROMPT_AUDIT_SKIP_OUTPUT", "true")

	c := newPromptAuditContext(t, 7, "alice")
	AuditPrompt(c, nil, PromptAuditDirectionInput, []byte("in"))
	AuditPrompt(c, nil, PromptAuditDirectionOutputChunk, []byte("out"))

	lines := readPromptAuditLines(t, filePath)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], `"direction":"input"`)
}
