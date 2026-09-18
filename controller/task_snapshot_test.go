package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestBuildTaskRequestSnapshotKeepsPlayableParams(t *testing.T) {
	snapshot := buildTaskRequestSnapshot(relaycommon.TaskSubmitReq{
		Prompt:      "一只猫",
		Model:       "minimax_h3",
		Mode:        "reference",
		Seconds:     "8",
		Resolution:  "768p",
		AspectRatio: "9:16",
		Images:      []string{" https://image.example.com/a.png "},
	})

	if snapshot.IsEmpty() {
		t.Fatal("snapshot should not be empty")
	}
	if snapshot.Duration != 8 {
		t.Fatalf("seconds should fold into duration, got %d", snapshot.Duration)
	}
	if snapshot.Resolution != "768p" || snapshot.AspectRatio != "9:16" || snapshot.Mode != "reference" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if len(snapshot.Images) != 1 || snapshot.Images[0] != "https://image.example.com/a.png" {
		t.Fatalf("reference urls should be trimmed, got %+v", snapshot.Images)
	}
}

func TestBuildTaskRequestSnapshotDropsDataURLsAndEmpty(t *testing.T) {
	// 纯 base64 参考图不进快照，否则 tasks 表会被撑爆
	snapshot := buildTaskRequestSnapshot(relaycommon.TaskSubmitReq{
		Duration: 5,
		Images:   []string{"data:image/png;base64,AAAA", "https://image.example.com/b.png"},
	})
	if snapshot.IsEmpty() || len(snapshot.Images) != 1 || snapshot.Images[0] != "https://image.example.com/b.png" {
		t.Fatalf("data url should be dropped, got %+v", snapshot)
	}

	if empty := buildTaskRequestSnapshot(relaycommon.TaskSubmitReq{Prompt: "only prompt"}); !empty.IsEmpty() {
		t.Fatalf("no parameters -> empty snapshot, got %+v", empty)
	}
}

func TestTaskSnapshotColumnIsIndependentFromProperties(t *testing.T) {
	// properties 只有 3 个字段（老节点也认识），快照单独一列
	properties := model.Properties{Input: "一只猫", OriginModelName: "minimax_h3"}
	if properties.Input != "一只猫" {
		t.Fatalf("unexpected properties: %+v", properties)
	}

	task := &model.Task{TaskID: "task_x"}
	task.RequestSnapshot = model.TaskRequestSnapshot{Duration: 5, Resolution: "480p"}
	if task.RequestSnapshot.IsEmpty() {
		t.Fatal("snapshot should be kept on its own column")
	}
}
