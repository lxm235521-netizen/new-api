package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonRawMessageToString(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want string
	}{
		{
			name: "object",
			data: json.RawMessage(`{"city":"Paris","days":0,"strict":false}`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "string",
			data: json.RawMessage(`"{\"city\":\"Paris\",\"days\":0,\"strict\":false}"`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "null",
			data: json.RawMessage(`null`),
			want: "",
		},
		{
			name: "empty",
			data: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, JsonRawMessageToString(tt.data))
		})
	}
}

func TestReplaceTaskVideoURLs(t *testing.T) {
	input := json.RawMessage(`{
		"object":"video.generation",
		"model":"grok-imagine-video-1.5",
		"url":"https://upstream.example/video.mp4",
		"download_url":"https://upstream.example/video.mp4",
		"video_url":"https://upstream.example/video.mp4",
		"video":{"url":"https://upstream.example/video.mp4"},
		"input":{"url":"https://input.example/image.jpg"}
	}`)
	proxyURL := "https://api.example.com/v1/videos/task_test/content"

	result := ReplaceTaskVideoURLs(input, proxyURL)
	var payload map[string]any
	require.NoError(t, Unmarshal(result, &payload))
	require.Equal(t, proxyURL, payload["url"])
	require.Equal(t, proxyURL, payload["download_url"])
	require.Equal(t, proxyURL, payload["video_url"])
	require.Equal(t, proxyURL, payload["video"].(map[string]any)["url"])
	require.Equal(t, "https://input.example/image.jpg", payload["input"].(map[string]any)["url"])

	objectInput := json.RawMessage(`{"object":"https://upstream.example/video.mp4","status":"SUCCEEDED","task_id":"task_test"}`)
	objectResult := ReplaceTaskVideoURLs(objectInput, proxyURL)
	var objectPayload map[string]any
	require.NoError(t, Unmarshal(objectResult, &objectPayload))
	require.Equal(t, proxyURL, objectPayload["object"])

	standardObject := json.RawMessage(`{"object":"video","status":"completed"}`)
	standardResult := ReplaceTaskVideoURLs(standardObject, proxyURL)
	var standardPayload map[string]any
	require.NoError(t, Unmarshal(standardResult, &standardPayload))
	require.Equal(t, "video", standardPayload["object"])
}
