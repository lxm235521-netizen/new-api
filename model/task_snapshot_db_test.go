package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 提交参数快照存在独立列 request_snapshot 上，必须能落库并读回
func TestTaskInsertKeepsRequestSnapshot(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID:     "task_snapshot_roundtrip",
		UserId:     1,
		Platform:   "1",
		Status:     TaskStatusInProgress,
		Progress:   "30%",
		SubmitTime: 123456,
		Properties: Properties{
			Input:             "一只猫",
			UpstreamModelName: "minimax_h3",
			OriginModelName:   "minimax_h3",
		},
		RequestSnapshot: TaskRequestSnapshot{
			Duration:    5,
			Resolution:  "480p",
			AspectRatio: "16:9",
			Images:      []string{"https://image.wgspai.cn/images/x.png"},
		},
	}
	require.NoError(t, task.Insert())

	var loaded Task
	require.NoError(t, DB.Where("task_id = ?", "task_snapshot_roundtrip").First(&loaded).Error)

	assert.Equal(t, "一只猫", loaded.Properties.Input)
	assert.False(t, loaded.RequestSnapshot.IsEmpty(), "快照在落库时丢了")
	assert.Equal(t, 5, loaded.RequestSnapshot.Duration)
	assert.Equal(t, "480p", loaded.RequestSnapshot.Resolution)
	assert.Equal(t, []string{"https://image.wgspai.cn/images/x.png"}, loaded.RequestSnapshot.Images)

	// UpdateWithStatus 是轮询用的全字段更新，也不能把快照写没
	loaded.Progress = "60%"
	won, err := loaded.UpdateWithStatus(TaskStatusInProgress)
	require.NoError(t, err)
	assert.True(t, won)

	var again Task
	require.NoError(t, DB.Where("task_id = ?", "task_snapshot_roundtrip").First(&again).Error)
	assert.False(t, again.RequestSnapshot.IsEmpty(), "UpdateWithStatus 把快照写没了")
	assert.Equal(t, 5, again.RequestSnapshot.Duration)
}

// 集群版本不一致时的保护：旧版本节点的模型里没有 request_snapshot 列，
// 它的轮询回写既不会覆盖快照，也不会把 properties 里的老字段弄丢。
func TestUpdateWithStatusDoesNotClobberSnapshot(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID:          "task_skew_protect",
		UserId:          1,
		Platform:        "1",
		Status:          TaskStatusInProgress,
		Progress:        "30%",
		SubmitTime:      123456,
		Properties:      Properties{Input: "一只猫"},
		RequestSnapshot: TaskRequestSnapshot{Duration: 5, Resolution: "480p"},
	}
	require.NoError(t, task.Insert())

	// 模拟旧版本节点：它只有一个 properties 字段，看不到 request_snapshot
	stale := &Task{}
	require.NoError(t, DB.Where("task_id = ?", "task_skew_protect").First(stale).Error)
	stale.Properties = Properties{Input: stale.Properties.Input}
	stale.RequestSnapshot = TaskRequestSnapshot{}
	stale.Status = TaskStatusSuccess

	won, err := stale.UpdateWithStatus(TaskStatusInProgress)
	require.NoError(t, err)
	assert.True(t, won)

	var after Task
	require.NoError(t, DB.Where("task_id = ?", "task_skew_protect").First(&after).Error)
	assert.Equal(t, string(TaskStatusSuccess), string(after.Status), "状态应该被更新")
	assert.False(t, after.RequestSnapshot.IsEmpty(), "快照不该被轮询覆盖")
	assert.Equal(t, "480p", after.RequestSnapshot.Resolution)
	assert.Equal(t, "一只猫", after.Properties.Input, "properties 也不该被覆盖")
}
