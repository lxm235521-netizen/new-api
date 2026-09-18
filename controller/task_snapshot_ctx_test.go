package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

// 复刻生产路径：gin 上下文 + 可重复读的 body storage + 适配器校验
func TestValidateMultipartDirectStoresFullPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"minimax_h3","prompt":"快照自检","seconds":"5",` +
		`"resolution":"480p","aspect_ratio":"16:9",` +
		`"images":["https://image.wgspai.cn/images/x.png"]}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	if taskErr := relaycommon.ValidateMultipartDirect(c, info); taskErr != nil {
		t.Fatalf("validate failed: %v", taskErr.Message)
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		t.Fatalf("get task request failed: %v", err)
	}
	t.Logf("stored: model=%q seconds=%q duration=%d resolution=%q ratio=%q images=%v action=%q",
		req.Model, req.Seconds, req.Duration, req.Resolution, req.AspectRatio, req.Images, info.Action)

	snapshot := buildTaskRequestSnapshot(req)
	if snapshot.IsEmpty() {
		t.Fatal("snapshot is empty —— context 里存的请求缺参数")
	}
	if snapshot.Resolution != "480p" || snapshot.Duration != 5 || len(snapshot.Images) != 1 {
		t.Fatalf("snapshot mismatch: %+v", snapshot)
	}
}

// 快照列（value receiver 的 Valuer）必须能落库、能读回
func TestRequestSnapshotValuerRoundTrip(t *testing.T) {
	snapshot := model.TaskRequestSnapshot{
		Duration:    5,
		Resolution:  "480p",
		AspectRatio: "16:9",
		Images:      []string{"https://image.wgspai.cn/images/x.png"},
	}

	value, err := snapshot.Value()
	if err != nil {
		t.Fatalf("value failed: %v", err)
	}
	raw, ok := value.([]byte)
	if !ok {
		t.Fatalf("unexpected value type: %T", value)
	}
	t.Logf("column value: %s", string(raw))

	var restored model.TaskRequestSnapshot
	if err := restored.Scan(raw); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if restored.Resolution != "480p" || restored.Duration != 5 || len(restored.Images) != 1 {
		t.Fatalf("snapshot lost in round trip: %+v", restored)
	}

	// 空快照写成 NULL，避免 tasks 表塞一堆空 JSON
	emptyValue, err := model.TaskRequestSnapshot{}.Value()
	if err != nil || emptyValue != nil {
		t.Fatalf("empty snapshot should be NULL, got %v (%v)", emptyValue, err)
	}

	// properties 里不该再塞快照（老节点会整列回写把它抹掉）
	propertiesRaw, err := model.Properties{Input: "x"}.Value()
	if err != nil {
		t.Fatalf("properties value failed: %v", err)
	}
	bytesValue, ok := propertiesRaw.([]byte)
	if !ok {
		t.Fatalf("unexpected properties value type: %T", propertiesRaw)
	}
	if strings.Contains(string(bytesValue), "request") {
		t.Fatalf("properties 不该包含 request: %s", string(bytesValue))
	}
}
