package meaicc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

type mediaItem struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type inputPayload struct {
	Prompt string      `json:"prompt,omitempty"`
	Media  []mediaItem `json:"media,omitempty"`
}

type parametersPayload struct {
	Resolution   string `json:"resolution,omitempty"`
	Duration     *int   `json:"duration,omitempty"`
	PromptExtend *bool  `json:"prompt_extend,omitempty"`
	Watermark    *bool  `json:"watermark,omitempty"`
	Ratio        string `json:"ratio,omitempty"`
}

type requestPayload struct {
	Model      string             `json:"model"`
	Input      inputPayload       `json:"input"`
	Parameters *parametersPayload `json:"parameters,omitempty"`
}

type responsePayload struct {
	ID        string `json:"id"`
	TaskID    string `json:"task_id,omitempty"`
	Object    string `json:"object,omitempty"`
	Model     string `json:"model,omitempty"`
	Status    string `json:"status"`
	Progress  int    `json:"progress,omitempty"`
	Seconds   int    `json:"seconds,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
	Error     *struct {
		Message string `json:"message,omitempty"`
		Code    string `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

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

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	req, err := parseTaskSubmitRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}

	normalizeImageFields(&req)
	if req.Metadata == nil {
		req.Metadata = map[string]interface{}{}
	}

	action := constant.TaskActionTextGenerate
	if req.HasImage() {
		action = constant.TaskActionGenerate
	}
	info.Action = action
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	duration := requestDuration(req)
	if duration <= 0 {
		duration = 5
	}
	return map[string]float64{"seconds": float64(duration)}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	body := convertToRequestPayload(req, info)
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var mResp responsePayload
	if err := common.Unmarshal(responseBody, &mResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	upstreamID := firstNonEmpty(mResp.ID, mResp.TaskID)
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.Model = info.OriginModelName
	ov.Status = meaiccStatusToVideoStatus(mResp.Status)
	ov.Progress = mResp.Progress
	if mResp.CreatedAt > 0 {
		ov.CreatedAt = mResp.CreatedAt
	} else {
		ov.CreatedAt = time.Now().Unix()
	}
	c.JSON(http.StatusOK, ov)
	return upstreamID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var resTask responsePayload
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := &relaycommon.TaskInfo{
		Code:     0,
		TaskID:   firstNonEmpty(resTask.ID, resTask.TaskID),
		Progress: formatProgress(resTask.Progress),
	}

	switch normalizeStatus(resTask.Status) {
	case "queued", "pending", "created", "submitted":
		taskResult.Status = model.TaskStatusQueued
		if taskResult.Progress == "" {
			taskResult.Progress = taskcommon.ProgressQueued
		}
	case "running", "processing", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
		if taskResult.Progress == "" {
			taskResult.Progress = taskcommon.ProgressInProgress
		}
	case "succeeded", "success", "completed":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = taskcommon.ProgressComplete
		taskResult.Url = strings.TrimSpace(resTask.Object)
	case "failed", "failure", "cancelled", "canceled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = taskcommon.ProgressComplete
		taskResult.Reason = "task failed"
		if resTask.Error != nil && strings.TrimSpace(resTask.Error.Message) != "" {
			taskResult.Reason = resTask.Error.Message
		}
	default:
		return nil, fmt.Errorf("unknown task status: %s", resTask.Status)
	}

	return taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var mResp responsePayload
	_ = common.Unmarshal(originTask.Data, &mResp)

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Model = originTask.Properties.OriginModelName
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	if mResp.CreatedAt > 0 {
		openAIVideo.CreatedAt = mResp.CreatedAt
	}
	openAIVideo.CompletedAt = originTask.UpdatedAt
	if resultURL := firstNonEmpty(mResp.Object, originTask.GetResultURL()); resultURL != "" {
		openAIVideo.SetMetadata("url", resultURL)
	}
	if originTask.Status == model.TaskStatusFailure && mResp.Error != nil {
		openAIVideo.Error = &dto.OpenAIVideoError{Message: mResp.Error.Message, Code: mResp.Error.Code}
	}

	return common.Marshal(openAIVideo)
}

func parseTaskSubmitRequest(c *gin.Context) (relaycommon.TaskSubmitReq, error) {
	var nativeReq requestPayload
	if err := common.UnmarshalBodyReusable(c, &nativeReq); err == nil && isNativeRequest(nativeReq) {
		metadata := nativeParametersToMetadata(nativeReq.Parameters)
		req := relaycommon.TaskSubmitReq{
			Prompt:   strings.TrimSpace(nativeReq.Input.Prompt),
			Model:    strings.TrimSpace(nativeReq.Model),
			Images:   extractMediaURLs(nativeReq.Input.Media),
			Metadata: metadata,
		}
		if nativeReq.Parameters != nil && nativeReq.Parameters.Duration != nil {
			req.Duration = *nativeReq.Parameters.Duration
		}
		return req, nil
	}

	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return req, err
	}
	return req, nil
}

func convertToRequestPayload(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) requestPayload {
	modelName := firstNonEmpty(info.UpstreamModelName, req.Model, "sd-2-c2")
	return requestPayload{
		Model: modelName,
		Input: inputPayload{
			Prompt: req.Prompt,
			Media:  requestImagesToMedia(req),
		},
		Parameters: parametersFromRequest(req),
	}
}

func parametersFromRequest(req relaycommon.TaskSubmitReq) *parametersPayload {
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	duration := requestDuration(req)
	if duration <= 0 {
		duration = 5
	}
	promptExtend := false
	watermark := false
	params := &parametersPayload{
		Resolution:   firstNonEmpty(stringMetadata(metadata, "resolution"), sizeToResolution(req.Size), "720P"),
		Duration:     &duration,
		PromptExtend: &promptExtend,
		Watermark:    &watermark,
		Ratio:        firstNonEmpty(stringMetadata(metadata, "ratio"), sizeToRatio(req.Size), "16:9"),
	}
	if v, ok := boolMetadata(metadata, "prompt_extend"); ok {
		params.PromptExtend = &v
	}
	if v, ok := boolMetadata(metadata, "watermark"); ok {
		params.Watermark = &v
	}
	return params
}

func normalizeImageFields(req *relaycommon.TaskSubmitReq) {
	if req.InputReference != "" {
		req.Images = append([]string{req.InputReference}, req.Images...)
	}
	if len(req.Images) == 0 && req.Image != "" {
		req.Images = []string{req.Image}
	}
}

func requestImagesToMedia(req relaycommon.TaskSubmitReq) []mediaItem {
	images := append([]string{}, req.Images...)
	if len(images) == 0 && req.Image != "" {
		images = append(images, req.Image)
	}
	media := make([]mediaItem, 0, len(images))
	for _, image := range images {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		media = append(media, mediaItem{Type: "reference_image", URL: image})
	}
	return media
}

func requestDuration(req relaycommon.TaskSubmitReq) int {
	if req.Duration > 0 {
		return req.Duration
	}
	if seconds, err := strconv.Atoi(req.Seconds); err == nil {
		return seconds
	}
	return 0
}

func extractMediaURLs(media []mediaItem) []string {
	urls := make([]string, 0, len(media))
	for _, item := range media {
		if url := strings.TrimSpace(item.URL); url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

func nativeParametersToMetadata(params *parametersPayload) map[string]interface{} {
	metadata := map[string]interface{}{}
	if params == nil {
		return metadata
	}
	if params.Resolution != "" {
		metadata["resolution"] = params.Resolution
	}
	if params.Ratio != "" {
		metadata["ratio"] = params.Ratio
	}
	if params.PromptExtend != nil {
		metadata["prompt_extend"] = *params.PromptExtend
	}
	if params.Watermark != nil {
		metadata["watermark"] = *params.Watermark
	}
	return metadata
}

func isNativeRequest(req requestPayload) bool {
	return strings.TrimSpace(req.Input.Prompt) != "" || len(req.Input.Media) > 0
}

func meaiccStatusToVideoStatus(status string) string {
	switch normalizeStatus(status) {
	case "queued", "pending", "created", "submitted":
		return dto.VideoStatusQueued
	case "running", "processing", "in_progress":
		return dto.VideoStatusInProgress
	case "succeeded", "success", "completed":
		return dto.VideoStatusCompleted
	case "failed", "failure", "cancelled", "canceled":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusQueued
	}
}

func normalizeStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func formatProgress(progress int) string {
	if progress <= 0 || progress >= 100 {
		return ""
	}
	return fmt.Sprintf("%d%%", progress)
}

func stringMetadata(metadata map[string]interface{}, key string) string {
	if raw, ok := metadata[key]; ok {
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
}

func boolMetadata(metadata map[string]interface{}, key string) (bool, bool) {
	raw, ok := metadata[key]
	if !ok {
		return false, false
	}
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return parsed, err == nil
	default:
		return false, false
	}
}

func sizeToResolution(size string) string {
	size = strings.ToLower(strings.TrimSpace(size))
	switch {
	case strings.Contains(size, "1080"):
		return "1080P"
	case strings.Contains(size, "768"):
		return "768P"
	case strings.Contains(size, "720"):
		return "720P"
	case strings.Contains(size, "512"):
		return "512P"
	default:
		return ""
	}
}

func sizeToRatio(size string) string {
	size = strings.ToLower(strings.TrimSpace(size))
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return ""
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return ""
	}
	switch {
	case width == height:
		return "1:1"
	case width > height:
		return "16:9"
	default:
		return "9:16"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
