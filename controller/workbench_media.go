package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	// workbenchMediaMaxBytes 单个素材的上限。前端 constants.js 按 50MB 卡，
	// 这里留出 multipart 封装开销的余量。
	workbenchMediaMaxBytes = 64 << 20
	// workbenchMediaFormField 转发给图床时用的表单字段名。
	// 契约固定为 file，换图床也不改。
	workbenchMediaFormField = "file"
	// workbenchMediaResponseMaxBytes 读取图床响应体的上限，防止对端返回超大内容。
	workbenchMediaResponseMaxBytes = 1 << 20
	// workbenchMediaTimeout 单次转发的整体超时。
	workbenchMediaTimeout = 5 * time.Minute
)

// workbenchMediaFieldCandidates 依次尝试的表单字段名。
// 前端固定发 file，其余是为了兼容直接拿 curl / 其它客户端打这个口的情况。
var workbenchMediaFieldCandidates = []string{"file", "file[]", "image", "image[]", "video", "audio"}

// mediaQuoteEscaper 转义 multipart 头部里的文件名，
// 与 net/http 内部 escapeQuotes 的行为一致。
var mediaQuoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, `\"`)

// UploadWorkbenchMedia 代理工作台参考素材上传。
//
// 为什么不由前端直传图床：files.mmg.lat 不返回任何 CORS 头（预检直接 405），
// 浏览器直传必被拦下；顺带的好处是图床 Token 只留在服务端，不下发到浏览器。
//
// 前端契约保持不变：POST multipart/form-data，字段 file=<二进制>，
// 成功返回 {success:true, data:{url, filename, size}}。
func UploadWorkbenchMedia(c *gin.Context) {
	uploadSetting := operation_setting.GetWorkbenchSetting().MediaUpload
	if strings.TrimSpace(uploadSetting.Endpoint) == "" {
		common.ApiErrorMsg(c, "图床未配置，请联系管理员")
		return
	}

	if c.Request.ContentLength > workbenchMediaMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"success": false,
			"message": "文件过大",
		})
		return
	}

	fileHeader := pickWorkbenchMediaFile(c)
	if fileHeader == nil {
		common.ApiErrorMsg(c, "未找到上传的文件")
		return
	}
	if fileHeader.Size > workbenchMediaMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"success": false,
			"message": "文件过大",
		})
		return
	}

	url, err := forwardWorkbenchMedia(c, uploadSetting, fileHeader)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	common.ApiSuccess(c, gin.H{
		"url":      url,
		"filename": fileHeader.Filename,
		"size":     fileHeader.Size,
	})
}

// pickWorkbenchMediaFile 按固定优先级取第一个上传的文件。
// 不直接遍历 MultipartForm.File：map 迭代顺序随机，多字段同时存在时结果不稳定。
func pickWorkbenchMediaFile(c *gin.Context) *multipart.FileHeader {
	form, err := c.MultipartForm()
	if err != nil || form == nil {
		return nil
	}
	for _, name := range workbenchMediaFieldCandidates {
		if headers := form.File[name]; len(headers) > 0 {
			return headers[0]
		}
	}
	for _, headers := range form.File {
		if len(headers) > 0 {
			return headers[0]
		}
	}
	return nil
}

// forwardWorkbenchMedia 把素材转发给图床，返回公开 URL。
func forwardWorkbenchMedia(
	c *gin.Context,
	uploadSetting operation_setting.WorkbenchMediaUpload,
	fileHeader *multipart.FileHeader,
) (string, error) {
	src, err := fileHeader.Open()
	if err != nil {
		return "", errors.New("读取上传文件失败")
	}
	defer src.Close()

	// 整个 body 在内存里拼好再发：这样能给出准确的 Content-Length，
	// 避免 chunked 上传被部分反代/WAF 拒绝。
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
		workbenchMediaFormField, mediaQuoteEscaper.Replace(fileHeader.Filename)))
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header.Set("Content-Type", contentType)

	part, err := writer.CreatePart(header)
	if err != nil {
		return "", errors.New("构造上传请求失败")
	}
	// LimitReader 兜底：fileHeader.Size 与真实长度不符时不至于无限读。
	if _, err := io.Copy(part, io.LimitReader(src, workbenchMediaMaxBytes+1)); err != nil {
		return "", errors.New("读取上传文件失败")
	}
	if body.Len() > workbenchMediaMaxBytes {
		return "", errors.New("文件过大")
	}
	formContentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		return "", errors.New("构造上传请求失败")
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), workbenchMediaTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadSetting.Endpoint, &body)
	if err != nil {
		return "", errors.New("构造上传请求失败")
	}
	req.ContentLength = int64(body.Len())
	req.Header.Set("Content-Type", formContentType)
	req.Header.Set("Accept", "*/*")
	if token := strings.TrimSpace(uploadSetting.Token); token != "" {
		// 原样发送：Zipline 用的是 `zk_xxx` 而不是 Bearer Token。
		req.Header.Set("Authorization", token)
	}
	if uploadSetting.PlainTextResponse {
		req.Header.Set("x-zipline-no-json", "true")
	}

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("图床连接失败：%v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, workbenchMediaResponseMaxBytes))
	if err != nil {
		return "", errors.New("读取图床响应失败")
	}

	url := extractWorkbenchMediaURL(respBody)
	if url == "" || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("图床返回失败（HTTP %d）：%s",
			resp.StatusCode, describeWorkbenchMediaError(respBody))
	}
	return url, nil
}

// extractWorkbenchMediaURL 从图床响应里取出 URL。
//
// 两种形态都要认：
//   - 纯文本：带 x-zipline-no-json 时 Zipline 直接回一行 https://.../u/xxx.png
//   - JSON：  {"url":...} / {"data":{"url":...}} / {"files":[{"url":...}]}
func extractWorkbenchMediaURL(respBody []byte) string {
	text := strings.TrimSpace(string(respBody))
	if isMediaURL(text) {
		return text
	}

	var payload struct {
		URL  string `json:"url"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
		Files []struct {
			URL string `json:"url"`
		} `json:"files"`
	}
	if err := common.Unmarshal(respBody, &payload); err != nil {
		return ""
	}
	for _, candidate := range []string{payload.URL, payload.Data.URL} {
		if trimmed := strings.TrimSpace(candidate); isMediaURL(trimmed) {
			return trimmed
		}
	}
	for _, file := range payload.Files {
		if trimmed := strings.TrimSpace(file.URL); isMediaURL(trimmed) {
			return trimmed
		}
	}
	return ""
}

func isMediaURL(value string) bool {
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return false
	}
	return !strings.ContainsAny(value, " \t\r\n")
}

// describeWorkbenchMediaError 把图床的错误响应整理成一行可读信息。
func describeWorkbenchMediaError(respBody []byte) string {
	text := strings.TrimSpace(string(respBody))
	if text == "" {
		return "无响应内容"
	}

	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(respBody, &payload); err == nil {
		if payload.Error != "" {
			return truncateRunes(payload.Error, 200)
		}
		if payload.Message != "" {
			return truncateRunes(payload.Message, 200)
		}
	}
	return truncateRunes(text, 200)
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
