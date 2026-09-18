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

package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// imageAsyncMaxDuration 后台图片生成的兜底超时。
// 上游是同步接口，正常几十秒出图；这里给足余量，超时按失败处理并退款。
const imageAsyncMaxDuration = 10 * time.Minute

// TaskPlaceholderContextKey 值非空表示「任务行已经落过占位行，中继结束时是补全而不是插入」
const TaskPlaceholderContextKey = "task_placeholder_id"

// imageAsyncMaxBodyMB 后台复制请求体时的上限（和网关的请求体上限保持一致）
const imageAsyncMaxBodyMB = 64

// imageRequestBody 是异步分发需要从请求体里读出来的信息
type imageRequestBody struct {
	raw    []byte
	async  bool
	model  string
	prompt string
}

// readImageRequestBody 读一次请求体，取出 async 标记和模型名，并把 body 回卷。
func readImageRequestBody(c *gin.Context) (imageRequestBody, error) {
	var result imageRequestBody

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return result, err
	}
	raw, err := storage.Bytes()
	if err != nil {
		return result, err
	}
	if _, seekErr := storage.Seek(0, io.SeekStart); seekErr != nil {
		return result, seekErr
	}
	c.Request.Body = io.NopCloser(storage)
	result.raw = raw

	var payload struct {
		Async  *bool  `json:"async,omitempty"`
		Model  string `json:"model,omitempty"`
		Prompt string `json:"prompt,omitempty"`
	}
	if err := common.Unmarshal(raw, &payload); err != nil {
		return result, err
	}
	result.async = payload.Async != nil && *payload.Async
	result.model = payload.Model
	result.prompt = payload.Prompt
	return result, nil
}

// createImageTaskPlaceholder 先落一条「排队中」的任务行。
//
// 异步图片要等上游几十秒才有结果，如果等结果出来才落库，用户在生成期间切走再切回来
// 就什么都看不到（本地乐观卡片随组件卸载没了）—— 所以提交时就先写一条占位行，
// 上游返回后再由中继按 task_id 补全（见 model.Task.UpdateFromRelayResult）。
func createImageTaskPlaceholder(c *gin.Context, taskID string, body imageRequestBody) *model.Task {
	var submitReq relaycommon.TaskSubmitReq
	_ = common.Unmarshal(body.raw, &submitReq)

	task := &model.Task{
		TaskID:     taskID,
		UserId:     c.GetInt("id"),
		Platform:   relay.TaskPlatformImage,
		Status:     model.TaskStatusQueued,
		Progress:   "0%",
		SubmitTime: time.Now().Unix(),
		// 给一个真实存在的渠道 ID：集群里版本不一致的旧节点轮询会先查渠道，
		// ID=0 会让它立刻把任务判死（实测 15 秒内）；非 0 则在「没有图片适配器」
		// 那一步安全退出（只记日志，不写库）。中继完成后用真实渠道覆盖它。
		ChannelId: model.GetAnyEnabledChannelID(),
		Properties: model.Properties{
			Input:             body.prompt,
			OriginModelName:   body.model,
			UpstreamModelName: body.model,
		},
		RequestSnapshot: buildTaskRequestSnapshot(submitReq),
	}
	if err := task.Insert(); err != nil {
		common.SysError("async image task: insert placeholder failed: " + err.Error())
		return nil
	}
	// 同时写进绘图日志：图片任务不该出现在「任务日志」里
	model.SyncImageTaskToDrawingLog(task)
	return task
}

// RelayImageGeneration 处理 POST /v1/images/generations。
//
// 带 "async": true 的请求走异步任务：立刻回一个 task_id，后台再调上游；
// 不带这个字段的请求完全走原来的同步中继 —— 行为、响应格式、计费都不变，
// 已经对接好的外部用户零影响（async 字段会被吃掉，不会转发给上游）。
func RelayImageGeneration(c *gin.Context) {
	body, bodyErr := readImageRequestBody(c)
	if bodyErr != nil || !body.async {
		// 读不出来或没开异步：交给原来的同步中继处理（它会重新读一次请求体）
		Relay(c, types.RelayFormatOpenAIImage)
		return
	}

	userID := c.GetInt("id")
	taskID := model.GenerateTaskID()
	createImageTaskPlaceholder(c, taskID, body)

	backgroundCtx, ctxErr := buildBackgroundRelayContext(c, taskID, body.raw)
	if ctxErr != nil {
		common.SysError("async image task: build background context failed: " + ctxErr.Error())
		Relay(c, types.RelayFormatOpenAIImage)
		return
	}

	// 先回执：客户端拿到的 id 就是最终落库的公开任务 ID
	c.JSON(http.StatusAccepted, gin.H{
		"id":         taskID,
		"task_id":    taskID,
		"object":     "task",
		"model":      body.model,
		"status":     "queued",
		"progress":   0,
		"created_at": time.Now().Unix(),
	})

	limit := operation_setting.GetWorkbenchSetting().AsyncImagePerUser
	gopool.Go(func() {
		// 每用户并发闸门：超出上限就在这里排队（不占用户连接）
		release := service.AcquireImageTaskSlot(userID, limit)
		defer release()
		defer backgroundCtx.cancel()

		// 走完整的任务链路：校验 → 计价 → 预扣费 → 调上游 → 补全任务行（SUCCESS/FAILURE）
		RelayTask(backgroundCtx.ctx)
		markPlaceholderFailedIfStuck(taskID, userID)
	})
}

type backgroundRelayContext struct {
	ctx    *gin.Context
	cancel context.CancelFunc
}

// markPlaceholderFailedIfStuck 兜底：中继早期就失败（没走到落库那一步）时，
// 占位行会一直挂在「排队中」。这里把它标成失败，免得用户看到永远转圈的任务。
func markPlaceholderFailedIfStuck(taskID string, userID int) {
	task, exist, err := model.GetByTaskId(userID, taskID)
	if err != nil || !exist || task == nil {
		return
	}
	if task.Status != model.TaskStatusQueued && task.Status != model.TaskStatusNotStart {
		return
	}
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FinishTime = time.Now().Unix()
	task.FailReason = "任务没有成功提交到上游，请重试"
	if updateErr := task.UpdateFromRelayResult(); updateErr != nil {
		common.SysError("async image task: mark placeholder failed error: " + updateErr.Error())
	}
	model.SyncImageTaskToDrawingLog(task)
}

// GetImageTask 查询异步图片任务状态（GET /v1/images/tasks/:task_id）。
//
// 图片上游是同步接口，没有可轮询的上游 id，所以这里只返回自己库里那条任务记录。
func GetImageTask(c *gin.Context) {
	taskID := c.Param("task_id")
	userID := c.GetInt("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "task_id is required"})
		return
	}

	task, exist, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "server_error", "message": err.Error()})
		return
	}
	if !exist || task == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "task_not_exist", "message": "task_not_exist"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          task.TaskID,
		"task_id":     task.TaskID,
		"object":      "image.task",
		"model":       task.Properties.OriginModelName,
		"status":      string(task.Status),
		"progress":    parseTaskProgress(task.Progress),
		"result_url":  task.GetResultURL(),
		"fail_reason": task.FailReason,
		"submit_time": task.SubmitTime,
		"finish_time": task.FinishTime,
	})
}

// parseTaskProgress 把 "100%" / "" 统一成 0-100 的整数，方便前端直接用
func parseTaskProgress(progress string) int {
	text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(progress), "%"))
	if text == "" {
		return 0
	}
	if value, err := strconv.Atoi(text); err == nil {
		if value < 0 {
			return 0
		}
		if value > 100 {
			return 100
		}
		return value
	}
	return 0
}

// buildBackgroundRelayContext 复制一份可脱手执行的 gin 上下文。
//
// 关键点：
//   - 用 context.Background() 派生，**不能**沿用 c.Request.Context() ——
//     否则客户端一断开，上游调用会被取消，钱花了图没了；
//   - 请求体要**重新复制一份**：原请求结束时 common.CleanupBodyStorage 会把
//     原来的 storage 关掉，后台协程再读就报 "body storage is closed"；
//   - 复制 c.Keys（里面有用户/令牌/渠道信息）；
//   - 响应写到 recorder 里（我们只关心结果，不需要发给客户端）；
//   - 显式声明 task_platform=image，图片任务才会走图片适配器。
func buildBackgroundRelayContext(c *gin.Context, taskID string, rawBody []byte) (*backgroundRelayContext, error) {
	if len(rawBody) == 0 {
		return nil, fmt.Errorf("empty request body")
	}

	storage, err := common.CreateBodyStorageFromReader(
		bytes.NewReader(rawBody),
		int64(len(rawBody)),
		int64(imageAsyncMaxBodyMB)<<20,
	)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), imageAsyncMaxDuration)

	recorder := httptest.NewRecorder()
	bg, _ := gin.CreateTestContext(recorder)
	if c.Request != nil {
		bg.Request = c.Request.Clone(ctx)
		bg.Request.Body = io.NopCloser(storage)
		bg.Request.ContentLength = int64(len(rawBody))
	} else {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, "/v1/images/generations", io.NopCloser(storage))
		if reqErr != nil {
			cancel()
			return nil, reqErr
		}
		bg.Request = req
	}
	bg.Params = c.Params
	for key, value := range c.Keys {
		bg.Set(key, value)
	}
	// 用新复制的 storage 覆盖原请求那份（原请求结束后就关了）
	bg.Set(common.KeyBodyStorage, storage)
	bg.Set(relay.TaskPlatformContextKey, string(relay.TaskPlatformImage))
	// 预生成公开任务 ID：回执给客户端的 id 与落库的 TaskID 保持一致
	bg.Set("task_public_id", taskID)
	// 已经落过占位行：中继结束时补全它，不要再插入一条
	bg.Set(TaskPlaceholderContextKey, taskID)

	return &backgroundRelayContext{ctx: bg, cancel: cancel}, nil
}
