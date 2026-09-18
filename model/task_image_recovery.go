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

package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

// imageTaskStaleSeconds 占位任务多久没动静就算残留（服务重启会杀掉后台协程）
const imageTaskStaleSeconds int64 = 600

// RecoverStaleImageTasks 启动时清理残留的图片任务。
//
// 图片任务的后台协程跟着进程走：如果生成过程中服务重启，那条「排队中」的占位行
// 就再也不会有人去补全了。这里在启动时把它们标成失败（附上明确原因），
// 免得用户看到永远转圈的任务。
//
// 注意：这类任务可能已经预扣过费用，但预扣信息写在任务行里（中继落库时才写），
// 重启发生在落库之前，因此这里无法自动退款 —— 下次任务正常结算即可。
func RecoverStaleImageTasks() {
	cutoff := common.GetTimestamp() - imageTaskStaleSeconds
	tasks := make([]*Task, 0)
	err := DB.Where("platform = ?", imageTaskPlatformFilter).
		Where("status NOT IN ?", []string{TaskStatusFailure, TaskStatusSuccess}).
		Where("submit_time < ?", cutoff).
		Limit(200).
		Find(&tasks).Error
	if err != nil {
		common.SysError("recover stale image tasks failed: " + err.Error())
		return
	}
	if len(tasks) == 0 {
		return
	}

	for _, task := range tasks {
		task.Status = TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = common.GetTimestamp()
		task.FailReason = "服务重启导致任务中断，请重新提交"
		if updateErr := task.UpdateFromRelayResult(); updateErr != nil {
			common.SysError("recover stale image task failed: " + updateErr.Error())
			continue
		}
		SyncImageTaskToDrawingLog(task)
	}
	common.SysLog(fmt.Sprintf("recovered stale image tasks: %d", len(tasks)))
}
