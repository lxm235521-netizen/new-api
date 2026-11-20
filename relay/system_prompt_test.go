package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGeminiSystemPromptRelayInfo(prompt string, override bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-2.5-flash",
			ChannelSetting: dto.ChannelSettings{
				SystemPrompt:         prompt,
				SystemPromptOverride: override,
			},
		},
	}
}

func TestApplySystemPromptIfNeededSurvivesGeminiConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := newGeminiSystemPromptRelayInfo("channel prompt", false)
	req := &dto.GeneralOpenAIRequest{
		Model: "gemini-2.5-flash",
		Messages: []dto.Message{
			{Role: "user", Content: "hi"},
		},
	}

	// This is what TextHelper now does before ConvertOpenAIRequest.
	applySystemPromptIfNeeded(c, info, req)

	geminiReq, err := gemini.CovertOpenAI2Gemini(c, *req, info)
	require.NoError(t, err)
	require.NotNil(t, geminiReq.SystemInstructions)
	require.Len(t, geminiReq.SystemInstructions.Parts, 1)
	require.Equal(t, "channel prompt", geminiReq.SystemInstructions.Parts[0].Text)
}

func TestApplySystemPromptIfNeededKeepsUserPromptPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := newGeminiSystemPromptRelayInfo("channel prompt", false)
	req := &dto.GeneralOpenAIRequest{
		Model: "gemini-2.5-flash",
		Messages: []dto.Message{
			{Role: "system", Content: "client system"},
			{Role: "user", Content: "hi"},
		},
	}

	applySystemPromptIfNeeded(c, info, req)

	geminiReq, err := gemini.CovertOpenAI2Gemini(c, *req, info)
	require.NoError(t, err)
	require.NotNil(t, geminiReq.SystemInstructions)
	require.Len(t, geminiReq.SystemInstructions.Parts, 1)
	require.Equal(t, "client system", geminiReq.SystemInstructions.Parts[0].Text)
}

func TestApplySystemPromptIfNeededConcatenatesWhenOverrideEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := newGeminiSystemPromptRelayInfo("channel prompt", true)
	req := &dto.GeneralOpenAIRequest{
		Model: "gemini-2.5-flash",
		Messages: []dto.Message{
			{Role: "system", Content: "client system"},
			{Role: "user", Content: "hi"},
		},
	}

	applySystemPromptIfNeeded(c, info, req)

	geminiReq, err := gemini.CovertOpenAI2Gemini(c, *req, info)
	require.NoError(t, err)
	require.NotNil(t, geminiReq.SystemInstructions)
	require.Len(t, geminiReq.SystemInstructions.Parts, 1)
	require.Equal(t, "channel prompt\nclient system", geminiReq.SystemInstructions.Parts[0].Text)
}

func TestApplySystemPromptIfNeededPreservesMultipartSystemContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := newGeminiSystemPromptRelayInfo("channel prompt", true)
	req := &dto.GeneralOpenAIRequest{
		Model: "gemini-2.5-flash",
		Messages: []dto.Message{
			{
				Role: "system",
				Content: []any{
					map[string]any{"type": "text", "text": "client system"},
				},
			},
			{Role: "user", Content: "hi"},
		},
	}

	applySystemPromptIfNeeded(c, info, req)

	// The parsed-content cache must be refreshed, otherwise Gemini's
	// StringContent-based conversion sees stale/empty content.
	geminiReq, err := gemini.CovertOpenAI2Gemini(c, *req, info)
	require.NoError(t, err)
	require.NotNil(t, geminiReq.SystemInstructions)
	require.Len(t, geminiReq.SystemInstructions.Parts, 1)
	require.Equal(t, "channel prompt\nclient system", geminiReq.SystemInstructions.Parts[0].Text)
}

func TestApplySystemPromptIfNeededPreservesMultipartSystemContentForClaude(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeAnthropic,
			UpstreamModelName: "claude-3-5-sonnet",
			ChannelSetting: dto.ChannelSettings{
				SystemPrompt:         "channel prompt",
				SystemPromptOverride: true,
			},
		},
	}
	req := &dto.GeneralOpenAIRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.Message{
			{
				Role: "system",
				Content: []any{
					map[string]any{"type": "text", "text": "client system"},
				},
			},
			{Role: "user", Content: "hi"},
		},
	}

	applySystemPromptIfNeeded(c, info, req)

	claudeReq, err := claude.RequestOpenAI2ClaudeMessage(c, *req)
	require.NoError(t, err)
	require.NotNil(t, claudeReq.System)
	system := claudeReq.ParseSystem()
	require.Len(t, system, 2)
	require.NotNil(t, system[0].Text)
	require.NotNil(t, system[1].Text)
	require.Equal(t, "channel prompt\n", *system[0].Text)
	require.Equal(t, "client system", *system[1].Text)
}

func TestApplySystemPromptIfNeededTreatsSystemAsExistingForDeveloperRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := newGeminiSystemPromptRelayInfo("channel prompt", false)
	info.UpstreamModelName = "o3-mini"
	req := &dto.GeneralOpenAIRequest{
		Model: "o3-mini",
		Messages: []dto.Message{
			{Role: "system", Content: "client system"},
			{Role: "user", Content: "hi"},
		},
	}

	applySystemPromptIfNeeded(c, info, req)

	require.Len(t, req.Messages, 2)
	require.Equal(t, "system", req.Messages[0].Role)
	require.Equal(t, "client system", req.Messages[0].StringContent())
}
