package controller

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// stubMediaBed 记录图床收到的请求，并按 plainText 决定响应形态。
type stubMediaBed struct {
	authorization string
	noJSONHeader  string
	contentLength int64
	fieldName     string
	fileName      string
	fileBody      []byte
}

// withMediaUploadSetting 临时替换图床配置。
func withMediaUploadSetting(t *testing.T, upload operation_setting.WorkbenchMediaUpload) {
	t.Helper()
	setting := operation_setting.GetWorkbenchSetting()
	original := setting.MediaUpload
	setting.MediaUpload = upload
	t.Cleanup(func() {
		setting.MediaUpload = original
	})
}

func newMultipartUploadRequest(t *testing.T, field, fileName string, payload []byte) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/workbench/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

type uploadResponseBody struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"data"`
}

// 代理必须把文件、鉴权头和 no-json 头原样转给图床，并把纯文本 URL 包装成标准响应。
func TestUploadWorkbenchMediaForwardsToImageBed(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)

	const wantURL = "https://files.mmg.lat/u/e3hPZL.png"
	payload := []byte("fake-png-bytes")
	bed := &stubMediaBed{}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bed.authorization = r.Header.Get("Authorization")
		bed.noJSONHeader = r.Header.Get("x-zipline-no-json")
		bed.contentLength = r.ContentLength

		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Errorf("upstream parse multipart: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for name, headers := range r.MultipartForm.File {
			if len(headers) == 0 {
				continue
			}
			bed.fieldName = name
			bed.fileName = headers[0].Filename
			file, err := headers[0].Open()
			if err != nil {
				t.Errorf("upstream open file: %v", err)
				return
			}
			defer file.Close()
			bed.fileBody, _ = io.ReadAll(file)
		}

		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(wantURL + "\n"))
	}))
	defer upstream.Close()

	withMediaUploadSetting(t, operation_setting.WorkbenchMediaUpload{
		Endpoint:          upstream.URL,
		Token:             "zk_test_token",
		PlainTextResponse: true,
	})

	engine := gin.New()
	engine.POST("/api/workbench/upload", UploadWorkbenchMedia)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, newMultipartUploadRequest(t, "file", "a.png", payload))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var body uploadResponseBody
	if err := common.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v\n%s", err, recorder.Body.String())
	}
	if !body.Success {
		t.Fatalf("success = false, message = %q", body.Message)
	}
	if body.Data.URL != wantURL {
		t.Fatalf("url = %q, want %q", body.Data.URL, wantURL)
	}
	if body.Data.Filename != "a.png" {
		t.Fatalf("filename = %q, want a.png", body.Data.Filename)
	}
	if body.Data.Size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", body.Data.Size, len(payload))
	}

	// 图床侧必须收到：原样的 Authorization（不带 Bearer）、no-json 头、file 字段、原始字节
	if bed.authorization != "zk_test_token" {
		t.Fatalf("upstream Authorization = %q, want raw token", bed.authorization)
	}
	if bed.noJSONHeader != "true" {
		t.Fatalf("upstream x-zipline-no-json = %q, want true", bed.noJSONHeader)
	}
	if bed.fieldName != "file" || bed.fileName != "a.png" {
		t.Fatalf("upstream field = %q / %q, want file / a.png", bed.fieldName, bed.fileName)
	}
	if !bytes.Equal(bed.fileBody, payload) {
		t.Fatalf("upstream file body = %q, want %q", bed.fileBody, payload)
	}
	// 给出准确的 Content-Length，避免 chunked 上传被反代/WAF 拒绝
	if bed.contentLength <= 0 {
		t.Fatalf("upstream ContentLength = %d, expected an explicit length", bed.contentLength)
	}
}

// 图床返回 JSON 时也要能取到 URL，并且能接受 file[] 这种字段名。
func TestUploadWorkbenchMediaParsesJSONResponseAndAlternateField(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)

	const wantURL = "https://files.mmg.lat/u/json-mode.png"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-zipline-no-json"); got != "" {
			t.Errorf("x-zipline-no-json should be omitted when disabled, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"files":[{"url":"` + wantURL + `","name":"json-mode.png"}]}`))
	}))
	defer upstream.Close()

	withMediaUploadSetting(t, operation_setting.WorkbenchMediaUpload{
		Endpoint:          upstream.URL,
		Token:             "",
		PlainTextResponse: false,
	})

	engine := gin.New()
	engine.POST("/api/workbench/upload", UploadWorkbenchMedia)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, newMultipartUploadRequest(t, "file[]", "b.png", []byte("x")))

	var body uploadResponseBody
	if err := common.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Success || body.Data.URL != wantURL {
		t.Fatalf("unexpected response: %+v (raw %s)", body, recorder.Body.String())
	}
}

// 图床报错时要把可读原因带回前端，且不能报 success。
func TestUploadWorkbenchMediaReportsUpstreamError(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"INVALID_KEY","error":"无效的 key"}`))
	}))
	defer upstream.Close()

	withMediaUploadSetting(t, operation_setting.WorkbenchMediaUpload{
		Endpoint:          upstream.URL,
		Token:             "zk_bad",
		PlainTextResponse: true,
	})

	engine := gin.New()
	engine.POST("/api/workbench/upload", UploadWorkbenchMedia)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, newMultipartUploadRequest(t, "file", "a.png", []byte("x")))

	var body uploadResponseBody
	if err := common.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Success {
		t.Fatal("success should be false when the image bed rejects the upload")
	}
	if body.Message == "" {
		t.Fatal("error message should be surfaced to the caller")
	}
	if !bytes.Contains([]byte(body.Message), []byte("无效的 key")) {
		t.Fatalf("message = %q, want the upstream error text", body.Message)
	}
}

// 没配图床时直接拒绝，不要发出请求。
func TestUploadWorkbenchMediaWithoutEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	withMediaUploadSetting(t, operation_setting.WorkbenchMediaUpload{Endpoint: "  "})

	engine := gin.New()
	engine.POST("/api/workbench/upload", UploadWorkbenchMedia)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, newMultipartUploadRequest(t, "file", "a.png", []byte("x")))

	var body uploadResponseBody
	if err := common.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Success {
		t.Fatal("success should be false when no endpoint is configured")
	}
}

func TestExtractWorkbenchMediaURL(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"plain text", "https://files.mmg.lat/u/a.png\n", "https://files.mmg.lat/u/a.png"},
		{"plain text with spaces", "  https://files.mmg.lat/u/a.png  ", "https://files.mmg.lat/u/a.png"},
		{"top level url", `{"url":"https://files.mmg.lat/u/a.png"}`, "https://files.mmg.lat/u/a.png"},
		{"nested data url", `{"data":{"url":"https://files.mmg.lat/u/a.png"}}`, "https://files.mmg.lat/u/a.png"},
		{"zipline files", `{"files":[{"url":"https://files.mmg.lat/u/a.png"}]}`, "https://files.mmg.lat/u/a.png"},
		{"relative url is not usable", `/u/a.png`, ""},
		{"empty", "", ""},
		{"html error page", `<html>502</html>`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractWorkbenchMediaURL([]byte(tc.body)); got != tc.want {
				t.Fatalf("extractWorkbenchMediaURL(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}
