package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 复刻工作台的提交体，确认解析 + 快照这条链路拿得到参数
func TestWorkbenchPayloadBecomesSnapshot(t *testing.T) {
	body := []byte(`{"model":"minimax_h3","prompt":"快照自检","seconds":"5",` +
		`"resolution":"480p","aspect_ratio":"16:9",` +
		`"images":["https://image.wgspai.cn/images/x.png"]}`)

	var req relaycommon.TaskSubmitReq
	if err := common.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	t.Logf("parsed: model=%q seconds=%q duration=%d resolution=%q ratio=%q images=%v",
		req.Model, req.Seconds, req.Duration, req.Resolution, req.AspectRatio, req.Images)

	snapshot := buildTaskRequestSnapshot(req)
	if snapshot.IsEmpty() {
		t.Fatal("snapshot is empty —— 参数没解析进去")
	}
	t.Logf("snapshot: %+v", snapshot)

	if snapshot.Resolution != "480p" || snapshot.AspectRatio != "16:9" || snapshot.Duration != 5 {
		t.Fatalf("snapshot mismatch: %+v", snapshot)
	}
	if len(snapshot.Images) != 1 {
		t.Fatalf("images lost: %+v", snapshot.Images)
	}
}
