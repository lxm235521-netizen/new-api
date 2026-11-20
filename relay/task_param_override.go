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
	"fmt"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// maxTaskParamOverrideBodyBytes 限制「构建后的请求体」参与参数覆盖的大小。
// 视频任务的请求体通常很小，但允许 base64 参考图，可能达到几十 MB；
// 超过上限时不改写请求体本身，模型映射仍会通过 resolveTaskModelOverride
// 在构建请求体之前完成。
const maxTaskParamOverrideBodyBytes = 8 << 20

// resolveTaskModelOverride 在构建请求体之前，用精简的请求投影评估渠道参数覆盖，
// 只把其中对 model 的改写同步回 RelayInfo。
//
// 背景：视频模型允许 base64 参考图，请求体可能远大于内存阈值；如果只在构建后的
// 请求体上做覆盖，超大请求体会被跳过，按分辨率映射模型就会失效。这里只投影少量
// 标量字段（不含 images/audios/videos），因此与请求体大小无关。
func resolveTaskModelOverride(c *gin.Context, info *relaycommon.RelayInfo) {
	if c == nil || info == nil || info.ChannelMeta == nil || len(info.ParamOverride) == 0 {
		return
	}
	if !relaycommon.ParamOverrideHasBodyOperations(info.ParamOverride) {
		return
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return
	}

	projection := map[string]interface{}{
		"model":        info.UpstreamModelName,
		"prompt":       req.Prompt,
		"resolution":   req.Resolution,
		"size":         req.Size,
		"seconds":      req.Seconds,
		"duration":     req.Duration,
		"aspect_ratio": req.AspectRatio,
		"metadata":     req.Metadata,
	}
	raw, err := common.Marshal(projection)
	if err != nil {
		return
	}
	overridden, err := relaycommon.ApplyParamOverride(
		raw,
		info.ParamOverride,
		relaycommon.BuildParamOverrideContext(info),
	)
	if err != nil {
		// 真正的报错留给构建后的正式覆盖阶段
		return
	}

	modelName := strings.TrimSpace(gjson.GetBytes(overridden, "model").String())
	if modelName == "" || modelName == info.UpstreamModelName {
		return
	}
	info.UpstreamModelName = modelName
	info.IsModelMapped = modelName != info.OriginModelName
	logger.LogDebug(c, "task upstream model overridden by param_override: %s -> %s", info.OriginModelName, modelName)
}

// applyTaskParamOverride 把渠道的「参数覆盖」应用到任务（视频/异步图片）请求体上。
//
// 只有 JSON 请求体参与覆盖；multipart 请求体（Sora 上传文件等）原样放行，
// 避免破坏分段编码。超过 maxTaskParamOverrideBodyBytes 的请求体原样放行
// （模型映射已在 resolveTaskModelOverride 中完成）。覆盖后如果上游 model 被改写，
// 会同步回 RelayInfo，让任务日志、轮询和后续调整都使用真正发给上游的模型名。
func applyTaskParamOverride(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (io.Reader, error) {
	if body == nil || info == nil || info.ChannelMeta == nil || len(info.ParamOverride) == 0 {
		return body, nil
	}
	if c == nil {
		return body, nil
	}
	// 只有 header 类操作时不用读取请求体
	if !relaycommon.ParamOverrideHasBodyOperations(info.ParamOverride) {
		return body, nil
	}
	contentType := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type")))
	if !strings.HasPrefix(contentType, "application/json") {
		return body, nil
	}
	// 客户端声明了大体积请求体时直接跳过覆盖，避免把大文件读进内存
	if c.Request != nil && c.Request.ContentLength > maxTaskParamOverrideBodyBytes {
		return body, nil
	}

	raw, err := io.ReadAll(io.LimitReader(body, maxTaskParamOverrideBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read task request body failed: %w", err)
	}
	if len(raw) == 0 {
		return bytes.NewReader(raw), nil
	}
	if len(raw) > maxTaskParamOverrideBodyBytes {
		// 没有声明长度但实际超限：不参与覆盖，把已读部分和剩余部分拼回原样，避免截断
		return io.MultiReader(bytes.NewReader(raw), body), nil
	}

	overridden, err := relaycommon.ApplyParamOverrideWithRelayInfo(raw, info)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(overridden, raw) {
		return bytes.NewReader(raw), nil
	}

	if modelName := strings.TrimSpace(gjson.GetBytes(overridden, "model").String()); modelName != "" && modelName != info.UpstreamModelName {
		info.UpstreamModelName = modelName
		info.IsModelMapped = modelName != info.OriginModelName
	}
	logger.LogDebug(c, "task request body overridden by param_override: %s", common.LocalLogPreview(string(overridden)))
	return bytes.NewReader(overridden), nil
}
