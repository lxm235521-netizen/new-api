package controller

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// videoProxyError returns a standardized OpenAI-style error response.
func videoProxyError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
		},
	})
}

// isImageTask 判断是不是「图片生成」任务。
//
// 图片走的是同一套任务表，但平台标识是 relay.TaskPlatformImage（渠道类型区分不了：
// 图片渠道也是 OpenAI 类型），结果地址直接存在 PrivateData.ResultURL 里。
func isImageTask(task *model.Task) bool {
	return string(task.Platform) == "image"
}

func VideoProxy(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "task_id is required")
		return
	}

	userID := c.GetInt("id")
	task, exists, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to query task %s: %s", taskID, err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}

	if task.Status != model.TaskStatusSuccess {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("Task is not completed yet, current status: %s", task.Status))
		return
	}

	channel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to get channel for task %s: %s", taskID, err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to retrieve channel information")
		return
	}
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	var videoURL string
	proxy := channel.GetSetting().Proxy
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create proxy client for task %s: %s", taskID, err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to create proxy client")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "", nil)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create request: %s", err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to create proxy request")
		return
	}

	// 播放器会用 Range 分段拉取（这批 mp4 的 moov 在文件尾部，必须能跳到末尾读），
	// 但这里**不能**把 Range 直接转发给「任务内容接口」——那个接口返回的是 JSON
	// 元数据（内含真正的媒体地址），带上 Range 只会拿到一段 JSON。
	// 所以先不带 Range 取元数据，解析出真实媒体地址后再把 Range 转发给媒体源
	// （对象存储支持 206；旧的单跳地址在下面单独重试一次）。
	rangeHeader := strings.TrimSpace(c.GetHeader("Range"))

	switch channel.Type {
	case constant.ChannelTypeGemini:
		apiKey := task.PrivateData.Key
		if apiKey == "" {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Missing stored API key for Gemini task %s", taskID))
			videoProxyError(c, http.StatusInternalServerError, "server_error", "API key not stored for task")
			return
		}
		videoURL, err = getGeminiVideoURL(channel, task, apiKey)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to resolve Gemini video URL for task %s: %s", taskID, err.Error()))
			videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to resolve Gemini video URL")
			return
		}
		req.Header.Set("x-goog-api-key", apiKey)
	case constant.ChannelTypeVertexAi:
		videoURL, err = getVertexVideoURL(channel, task)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to resolve Vertex video URL for task %s: %s", taskID, err.Error()))
			videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to resolve Vertex video URL")
			return
		}
	default:
		// 图片任务（同步上游）以及其它把最终地址存在 PrivateData.ResultURL 的平台：
		// 直接取存下来的地址。必须放在 OpenAI/Sora 分支之前判断 —— 图片渠道也是
		// OpenAI 类型，但它的结果不在上游的 /v1/videos/{id}/content 上。
		if isImageTask(task) {
			videoURL = task.GetResultURL()
			break
		}
		switch channel.Type {
		case constant.ChannelTypeOpenAI, constant.ChannelTypeSora:
			videoURL = fmt.Sprintf("%s/v1/videos/%s/content", baseURL, task.GetUpstreamTaskID())
			req.Header.Set("Authorization", "Bearer "+channel.Key)
		default:
			// Video URL is stored in PrivateData.ResultURL (fallback to FailReason for old data)
			videoURL = task.GetResultURL()
		}
	}

	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Video URL is empty for task %s", taskID))
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch video content")
		return
	}

	if strings.HasPrefix(videoURL, "data:") {
		if err := writeVideoDataURL(c, videoURL); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to decode video data URL for task %s: %s", taskID, err.Error()))
			videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch video content")
		}
		return
	}

	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(videoURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Video URL blocked for task %s: %v", taskID, err))
		videoProxyError(c, http.StatusForbidden, "server_error", fmt.Sprintf("request blocked: %v", err))
		return
	}

	req.URL, err = url.Parse(videoURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to parse URL %s: %s", videoURL, err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to create proxy request")
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to fetch video from %s: %s", videoURL, err.Error()))
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch video content")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Upstream returned status %d for %s", resp.StatusCode, videoURL))
		videoProxyError(c, http.StatusBadGateway, "server_error",
			fmt.Sprintf("Upstream service returned status %d", resp.StatusCode))
		return
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	streamBody := io.Reader(resp.Body)
	bufferedBody := bufio.NewReader(resp.Body)
	isJSON := strings.Contains(contentType, "application/json") || strings.Contains(contentType, "+json")
	if !isJSON {
		if prefix, peekErr := bufferedBody.Peek(512); peekErr == nil || len(prefix) > 0 {
			trimmed := bytes.TrimSpace(prefix)
			isJSON = len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
		}
		streamBody = bufferedBody
	}
	if isJSON {
		jsonBody, readErr := io.ReadAll(bufferedBody)
		if readErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to read JSON response for task %s: %s", taskID, readErr.Error()))
			videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch video content")
			return
		}
		if mediaURL := extractVideoURLFromJSON(jsonBody); mediaURL != "" {
			resp.Body.Close()
			originalHost := req.URL.Host
			videoURL = mediaURL
			if err := common.ValidateURLWithFetchSetting(videoURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("Video URL blocked for task %s: %v", taskID, err))
				videoProxyError(c, http.StatusForbidden, "server_error", fmt.Sprintf("request blocked: %v", err))
				return
			}
			req.URL, err = url.Parse(videoURL)
			if err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to parse media URL %s: %s", videoURL, err.Error()))
				videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to create proxy request")
				return
			}
			mediaReq := req.Clone(ctx)
			// 真实媒体源（对象存储 / CDN）支持 Range，转发给播放器用
			if rangeHeader != "" {
				mediaReq.Header.Set("Range", rangeHeader)
			}
			if mediaURLParsed, parseMediaErr := url.Parse(videoURL); parseMediaErr == nil && mediaURLParsed.Host != originalHost {
				mediaReq.Header.Del("Authorization")
				mediaReq.Header.Del("x-goog-api-key")
			}
			resp, err = client.Do(mediaReq)
			if err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to fetch video from %s: %s", videoURL, err.Error()))
				videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch video content")
				return
			}
			defer resp.Body.Close()
			streamBody = resp.Body
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
				logger.LogError(c.Request.Context(), fmt.Sprintf("Upstream returned status %d for %s", resp.StatusCode, videoURL))
				videoProxyError(c, http.StatusBadGateway, "server_error",
					fmt.Sprintf("Upstream service returned status %d", resp.StatusCode))
				return
			}
		}
		if mediaURL := extractVideoURLFromJSON(jsonBody); mediaURL == "" {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Upstream returned JSON without a video URL for task %s", taskID))
			videoProxyError(c, http.StatusBadGateway, "server_error", "Upstream did not return video content")
			return
		}
	}

	if !isJSON && rangeHeader != "" {
		// 回源本身就是媒体文件（没有 JSON 中转）：带 Range 再要一次，
		// 让播放器拿到 206，可以跳到文件尾部读 moov。
		retryReq := req.Clone(ctx)
		retryReq.Header.Set("Range", rangeHeader)
		if retryResp, retryErr := client.Do(retryReq); retryErr == nil {
			if retryResp.StatusCode == http.StatusPartialContent || retryResp.StatusCode == http.StatusOK {
				resp.Body.Close()
				resp = retryResp
				streamBody = resp.Body
				defer resp.Body.Close()
			} else {
				retryResp.Body.Close()
			}
		}
	}

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}

	c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(c.Writer, streamBody); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to stream video content: %s", err.Error()))
	}
}

func writeVideoDataURL(c *gin.Context, dataURL string) error {
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid data url")
	}

	header := parts[0]
	payload := parts[1]
	if !strings.HasPrefix(header, "data:") || !strings.Contains(header, ";base64") {
		return fmt.Errorf("unsupported data url")
	}

	mimeType := strings.TrimPrefix(header, "data:")
	mimeType = strings.TrimSuffix(mimeType, ";base64")
	if mimeType == "" {
		mimeType = "video/mp4"
	}

	videoBytes, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		videoBytes, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return err
		}
	}

	c.Writer.Header().Set("Content-Type", mimeType)
	c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
	c.Writer.WriteHeader(http.StatusOK)
	_, err = c.Writer.Write(videoBytes)
	return err
}

func extractVideoURLFromJSON(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return extractVideoURLFromMap(payload)
}

func extractVideoURLFromMap(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if url := extractStringField(payload, "url"); url != "" {
		return url
	}
	if url := extractStringField(payload, "download_url"); url != "" {
		return url
	}
	if url := extractStringField(payload, "video_url"); url != "" {
		return url
	}
	if video, ok := payload["video"].(map[string]any); ok {
		if url := extractVideoURLFromMap(video); url != "" {
			return url
		}
	}
	if videos, ok := payload["videos"].([]any); ok {
		for _, item := range videos {
			if vm, ok := item.(map[string]any); ok {
				if url := extractVideoURLFromMap(vm); url != "" {
					return url
				}
			}
		}
	}
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		if url := extractVideoURLFromMap(metadata); url != "" {
			return url
		}
	}
	if output, ok := payload["output"].(map[string]any); ok {
		if url := extractVideoURLFromMap(output); url != "" {
			return url
		}
	}
	if result, ok := payload["result"].(map[string]any); ok {
		if url := extractVideoURLFromMap(result); url != "" {
			return url
		}
	}
	return ""
}

func extractStringField(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if raw, ok := payload[key]; ok {
		switch v := raw.(type) {
		case string:
			return strings.TrimSpace(v)
		case map[string]any:
			if nested := extractStringField(v, "url"); nested != "" {
				return nested
			}
		}
	}
	return ""
}
