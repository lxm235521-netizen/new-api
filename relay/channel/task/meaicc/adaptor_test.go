package meaicc

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestConvertToRequestPayloadMapsFlatRequest(t *testing.T) {
	promptExtend := true
	watermark := false
	req := relaycommon.TaskSubmitReq{
		Prompt:   "@image1 and @image2 dancing",
		Model:    "sd-2-c2",
		Images:   []string{"https://example.com/a.png", "https://example.com/b.png"},
		Size:     "1280x720",
		Duration: 15,
		Metadata: map[string]interface{}{
			"prompt_extend": promptExtend,
			"watermark":      watermark,
		},
	}

	body := convertToRequestPayload(req, &relaycommon.RelayInfo{})

	require.Equal(t, "sd-2-c2", body.Model)
	require.Equal(t, req.Prompt, body.Input.Prompt)
	require.Len(t, body.Input.Media, 2)
	require.Equal(t, "reference_image", body.Input.Media[0].Type)
	require.Equal(t, "https://example.com/a.png", body.Input.Media[0].URL)
	require.Equal(t, "720P", body.Parameters.Resolution)
	require.Equal(t, "16:9", body.Parameters.Ratio)
	require.NotNil(t, body.Parameters.Duration)
	require.Equal(t, 15, *body.Parameters.Duration)
	require.NotNil(t, body.Parameters.PromptExtend)
	require.True(t, *body.Parameters.PromptExtend)
	require.NotNil(t, body.Parameters.Watermark)
	require.False(t, *body.Parameters.Watermark)
}

func TestParseTaskResultMapsMeaiccStatuses(t *testing.T) {
	adaptor := &TaskAdaptor{}

	running, err := adaptor.ParseTaskResult([]byte(`{"id":"task_1","status":"RUNNING","progress":23,"created_at":1785742369}`))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusInProgress, model.TaskStatus(running.Status))
	require.Equal(t, "23%", running.Progress)

	succeeded, err := adaptor.ParseTaskResult([]byte(`{"id":"task_1","object":"https://cdn.example.com/out.mp4","status":"SUCCEEDED","created_at":1785761507}`))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, model.TaskStatus(succeeded.Status))
	require.Equal(t, "100%", succeeded.Progress)
	require.Equal(t, "https://cdn.example.com/out.mp4", succeeded.Url)

	failed, err := adaptor.ParseTaskResult([]byte(`{"id":"task_1","object":"","status":"FAILED: poll status=451 code=reference_image_privacy_error message=The reference image contains a real person's face and cannot be used to generate content.","seconds":0,"created_at":1785769304}`))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusFailure, model.TaskStatus(failed.Status))
	require.Equal(t, "100%", failed.Progress)
	require.Equal(t, "poll status=451 code=reference_image_privacy_error message=The reference image contains a real person's face and cannot be used to generate content.", failed.Reason)
}

func TestConvertToOpenAIVideoUsesTaskResultURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: 1785761507,
		UpdatedAt: 1785762133,
		Data:      []byte(`{"id":"task_upstream","object":"https://cdn.example.com/out.mp4","status":"SUCCEEDED","created_at":1785761507}`),
	}
	task.Properties.OriginModelName = "sd-2-c2"

	data, err := adaptor.ConvertToOpenAIVideo(task)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(data, &payload))
	require.Equal(t, "task_public", payload["id"])
	require.Equal(t, "completed", payload["status"])
	require.Equal(t, "sd-2-c2", payload["model"])
	metadata := payload["metadata"].(map[string]interface{})
	require.Equal(t, "https://cdn.example.com/out.mp4", metadata["url"])
}
