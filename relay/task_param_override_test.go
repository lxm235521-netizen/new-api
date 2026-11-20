/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package relay

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// resolutionModelOverride 复刻「按分辨率映射上游模型」的配置：
// 同一个模型按分辨率映射到不同上游模型。
func resolutionModelOverride() map[string]interface{} {
	return map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "set",
				"value": "upstream-video-1080p-model",
				"logic": "AND",
				"conditions": []interface{}{
					map[string]interface{}{"path": "model", "mode": "full", "value": "video-model"},
					map[string]interface{}{"path": "resolution", "mode": "full", "value": "1080p"},
				},
			},
			map[string]interface{}{
				"path":  "model",
				"mode":  "set",
				"value": "upstream-video-768p-model",
				"logic": "AND",
				"conditions": []interface{}{
					map[string]interface{}{"path": "model", "mode": "full", "value": "video-model"},
					map[string]interface{}{"path": "resolution", "mode": "full", "value": "768p"},
				},
			},
		},
	}
}

func newTaskOverrideContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", bytes.NewReader(nil))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

// caseInsensitiveResolutionOverride 把分辨率条件改成 ieq（不区分大小写）。
func caseInsensitiveResolutionOverride() map[string]interface{} {
	override := resolutionModelOverride()
	ops := override["operations"].([]interface{})
	for _, raw := range ops {
		op := raw.(map[string]interface{})
		for _, rawCond := range op["conditions"].([]interface{}) {
			cond := rawCond.(map[string]interface{})
			if cond["path"] == "resolution" {
				cond["mode"] = "ieq"
			}
		}
	}
	return override
}

func TestResolveTaskModelOverrideWithoutBody(t *testing.T) {
	// 请求体可以是任意大（例如 base64 参考图），投影解析不需要读它
	cases := []struct {
		name       string
		resolution string
		wantModel  string
	}{
		{name: "lower 1080p", resolution: "1080p", wantModel: "upstream-video-1080p-model"},
		{name: "upper 1080P", resolution: "1080P", wantModel: "upstream-video-1080p-model"},
		{name: "mixed 768P", resolution: "768P", wantModel: "upstream-video-768p-model"},
		{name: "unmatched 720p", resolution: "720p", wantModel: "video-model"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTaskOverrideContext(t)
			c.Set("task_request", relaycommon.TaskSubmitReq{
				Model:      "video-model",
				Prompt:     "p",
				Seconds:    "10",
				Resolution: tc.resolution,
			})
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ParamOverride:     caseInsensitiveResolutionOverride(),
				UpstreamModelName: "video-model",
			}}
			info.OriginModelName = "video-model"

			resolveTaskModelOverride(c, info)

			require.Equal(t, tc.wantModel, info.UpstreamModelName)
			require.Equal(t, tc.wantModel != "video-model", info.IsModelMapped)
		})
	}
}

func TestResolveTaskModelOverrideSkipsHeaderOnlyOverride(t *testing.T) {
	c := newTaskOverrideContext(t)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:      "video-model",
		Resolution: "1080p",
	})
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ParamOverride: map[string]interface{}{
			"operations": []interface{}{
				map[string]interface{}{"mode": "set_header", "path": "x-test", "value": "1"},
			},
		},
		UpstreamModelName: "video-model",
	}}

	resolveTaskModelOverride(c, info)

	require.Equal(t, "video-model", info.UpstreamModelName)
	require.False(t, relaycommon.ParamOverrideHasBodyOperations(info.ParamOverride))
	require.True(t, relaycommon.ParamOverrideHasBodyOperations(caseInsensitiveResolutionOverride()))
}

func TestApplyTaskParamOverrideMapsModelByResolution(t *testing.T) {
	cases := []struct {
		name       string
		resolution string
		wantModel  string
	}{
		{name: "768p", resolution: "768p", wantModel: "upstream-video-768p-model"},
		{name: "1080p", resolution: "1080p", wantModel: "upstream-video-1080p-model"},
		{name: "720p unmatched", resolution: "720p", wantModel: "video-model"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTaskOverrideContext(t)
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ParamOverride:     resolutionModelOverride(),
					UpstreamModelName: "video-model",
				},
			}
			info.OriginModelName = "video-model"

			body := `{"model":"video-model","prompt":"p","resolution":"` + tc.resolution + `","seconds":"10"}`
			reader, err := applyTaskParamOverride(c, info, bytes.NewReader([]byte(body)))
			require.NoError(t, err)

			out, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, tc.wantModel, gjson.GetBytes(out, "model").String(), "body=%s", out)
			require.Equal(t, tc.resolution, gjson.GetBytes(out, "resolution").String())
			require.Equal(t, tc.wantModel, info.UpstreamModelName)

			if tc.wantModel == "video-model" {
				require.False(t, info.IsModelMapped)
			} else {
				require.True(t, info.IsModelMapped)
			}
		})
	}
}

func TestApplyTaskParamOverrideSkipsNonJSONAndEmptyOverride(t *testing.T) {
	payload := []byte(`{"model":"video-model","resolution":"768p"}`)

	t.Run("multipart passthrough", func(t *testing.T) {
		c := newTaskOverrideContext(t)
		c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride:     resolutionModelOverride(),
			UpstreamModelName: "video-model",
		}}
		reader, err := applyTaskParamOverride(c, info, bytes.NewReader(payload))
		require.NoError(t, err)
		out, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, payload, out)
		require.Equal(t, "video-model", info.UpstreamModelName)
	})

	t.Run("empty override", func(t *testing.T) {
		c := newTaskOverrideContext(t)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "video-model",
		}}
		reader, err := applyTaskParamOverride(c, info, bytes.NewReader(payload))
		require.NoError(t, err)
		out, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, payload, out)
	})

	t.Run("nil channel meta", func(t *testing.T) {
		c := newTaskOverrideContext(t)
		info := &relaycommon.RelayInfo{}
		reader, err := applyTaskParamOverride(c, info, bytes.NewReader(payload))
		require.NoError(t, err)
		out, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, payload, out)
	})
}

func TestApplyTaskParamOverrideLargeBodyNotTruncated(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ParamOverride:     resolutionModelOverride(),
		UpstreamModelName: "video-model",
	}}

	t.Run("declared large content length skips override", func(t *testing.T) {
		c := newTaskOverrideContext(t)
		c.Request.ContentLength = maxTaskParamOverrideBodyBytes + 1
		payload := []byte(`{"model":"video-model","resolution":"768p"}`)
		reader, err := applyTaskParamOverride(c, info, bytes.NewReader(payload))
		require.NoError(t, err)
		out, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, payload, out)
	})

	t.Run("unknown length over limit is forwarded intact", func(t *testing.T) {
		c := newTaskOverrideContext(t)
		c.Request.ContentLength = -1
		big := `{"model":"video-model","resolution":"768p","prompt":"` +
			strings.Repeat("a", maxTaskParamOverrideBodyBytes) + `"}`
		reader, err := applyTaskParamOverride(c, info, bytes.NewReader([]byte(big)))
		require.NoError(t, err)
		out, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Len(t, out, len(big))
		require.Equal(t, big, string(out))
	})
}

func TestApplyTaskParamOverrideCaseInsensitiveCondition(t *testing.T) {
	c := newTaskOverrideContext(t)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ParamOverride:     caseInsensitiveResolutionOverride(),
		UpstreamModelName: "video-model",
	}}
	info.OriginModelName = "video-model"

	body := `{"model":"video-model","resolution":"1080P","seconds":"10"}`
	reader, err := applyTaskParamOverride(c, info, bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	out, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "upstream-video-1080p-model", gjson.GetBytes(out, "model").String())
	require.Equal(t, "upstream-video-1080p-model", info.UpstreamModelName)
	require.True(t, info.IsModelMapped)
}
