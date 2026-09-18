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

// Package taskimage 实现「图片生成」的任务适配器。
//
// 和视频不同，图片上游是**同步**接口：一次 POST /v1/images/generations 直接返回
// {model, url}，没有 task_id 可以轮询。这里的做法是：
//
//  1. 校验/计价/预扣费/落库，全部复用视频任务那套链路（计费与历史记录天然一致）；
//  2. DoResponse 直接给出最终结果 URL，并把它标成「提交即完成」；
//  3. 控制器据此把任务落成 SUCCESS（或 FAILURE + 退款），不进轮询。
//
// 并发问题由上层解决：POST /v1/images/generations 带 "async": true 时，
// 先回 task_id、后台再跑这条链路，因此上游的慢不会占住用户连接。
package taskimage

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const ChannelName = "Image"

// ModelList 为空：可用模型完全由渠道配置决定
var ModelList = []string{}

// 按上游契约固定下来的参数（工作台不暴露这些开关）
const (
	imageQuality        = "high"
	imageCount          = 1
	imageResponseFormat = "url"
)

// 原始请求体存在上下文里：BuildRequestBody 要原样透传（images 里可能有 base64）
const ctxRawImageRequest = "image_task_raw_request"

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// SyncCompletedOnSubmit：图片是同步上游，提交即完成
func (a *TaskAdaptor) SyncCompletedOnSubmit() bool { return true }

// ValidateRequestAndSetAction 解析请求并确定 action。
//
// 先把原始 body 存下来（后面的 UnmarshalBodyReusable 会重新读，所以必须回卷），
// 再复用通用的任务请求校验（prompt/model 必填、参考图决定 action、写入任务快照）。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusBadRequest)
	}
	raw, err := storage.Bytes()
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusBadRequest)
	}
	if _, seekErr := storage.Seek(0, io.SeekStart); seekErr != nil {
		return service.TaskErrorWrapperLocal(seekErr, "read_request_body_failed", http.StatusBadRequest)
	}
	c.Request.Body = io.NopCloser(storage)
	c.Set(ctxRawImageRequest, raw)

	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/images/generations", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// BuildRequestBody 原样透传用户请求，只做三件事：
// 替换成上游模型名、补上固定参数、去掉我们自己的 async 开关。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	rawValue, exists := c.Get(ctxRawImageRequest)
	if !exists {
		return nil, errors.New("raw image request not found")
	}
	raw, ok := rawValue.([]byte)
	if !ok || len(raw) == 0 {
		return nil, errors.New("raw image request is empty")
	}

	var payload map[string]any
	if err := common.Unmarshal(raw, &payload); err != nil {
		return nil, errors.Wrap(err, "invalid request body")
	}

	payload["model"] = info.UpstreamModelName
	delete(payload, "async")
	payload["quality"] = imageQuality
	payload["n"] = imageCount
	payload["response_format"] = imageResponseFormat

	body, err := common.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request body failed")
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 读同步响应。成功时把结果 URL 写进上下文并返回；失败时也返回 nil error，
// 由控制器落成一条 FAILURE 任务（用户能在历史里看到失败原因）并退还预扣费。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	if resp == nil {
		c.Set(relaycommon.ContextKeyTaskFailReason, "上游没有返回内容")
		return "", nil, nil
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		c.Set(relaycommon.ContextKeyTaskFailReason, "读取上游响应失败："+err.Error())
		return "", nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		c.Set(relaycommon.ContextKeyTaskFailReason,
			fmt.Sprintf("上游返回 %d：%s", resp.StatusCode, summarizeBody(body)))
		return "", body, nil
	}

	resultURL := extractImageURL(body)
	if resultURL == "" {
		c.Set(relaycommon.ContextKeyTaskFailReason, "上游没有返回图片地址："+summarizeBody(body))
		return "", body, nil
	}

	c.Set(relaycommon.ContextKeyTaskResultURL, resultURL)
	return resultURL, body, nil
}

// 图片任务没有可轮询的上游 id，这两个方法只作为接口占位
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	return nil, errors.New("image task is synchronous, no polling")
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	if url := extractImageURL(respBody); url != "" {
		return &relaycommon.TaskInfo{
			Code:     0,
			Status:   model.TaskStatusSuccess,
			Url:      url,
			Progress: taskcommon.ProgressComplete,
		}, nil
	}
	return &relaycommon.TaskInfo{
		Code:     0,
		Status:   model.TaskStatusFailure,
		Reason:   summarizeBody(respBody),
		Progress: taskcommon.ProgressComplete,
	}, nil
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

// extractImageURL 兼容几种常见返回：
//
//	{"url": "https://..."}                        ← 你的网关
//	{"data": [{"url": "https://..."}]}            ← OpenAI 标准
//	{"data": [{"b64_json": "..."}]}               ← OpenAI base64
func extractImageURL(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if url := stringField(payload, "url"); url != "" {
		return url
	}
	if url := stringField(payload, "image_url"); url != "" {
		return url
	}
	if data, ok := payload["data"].([]any); ok {
		for _, item := range data {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if url := stringField(entry, "url"); url != "" {
				return url
			}
			if b64 := stringField(entry, "b64_json"); b64 != "" {
				return "data:image/png;base64," + b64
			}
		}
	}
	return ""
}

func stringField(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if value, ok := payload[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func summarizeBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	if text == "" {
		text = "(空响应)"
	}
	return text
}

var _ channel.TaskAdaptor = (*TaskAdaptor)(nil)
var _ channel.SyncTaskAdaptor = (*TaskAdaptor)(nil)

// 让 constant 包的 action 常量在本包可用（校验逻辑在 relaycommon 里设置 action）
var _ = constant.TaskActionGenerate
