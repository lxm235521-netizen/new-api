package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// UpdateTaskBulk 薄入口，实际轮询逻辑在 service 层
func UpdateTaskBulk() {
	service.TaskPollingLoop()
}

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:        constant.TaskPlatform(c.Query("platform")),
		TaskID:          c.Query("task_id"),
		Status:          c.Query("status"),
		Action:          c.Query("action"),
		StartTimestamp:  startTimestamp,
		EndTimestamp:    endTimestamp,
		ChannelID:       c.Query("channel_id"),
		ExcludePlatform: c.Query("exclude_platform"),
	}

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	pageInfo.SetTotal(int(taskTotal(items, pageInfo, func() int64 {
		return model.TaskCountAllTasks(queryParams)
	})))
	pageInfo.SetItems(tasksToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:        constant.TaskPlatform(c.Query("platform")),
		TaskID:          c.Query("task_id"),
		Status:          c.Query("status"),
		Action:          c.Query("action"),
		StartTimestamp:  startTimestamp,
		EndTimestamp:    endTimestamp,
		ExcludePlatform: c.Query("exclude_platform"),
	}

	items := model.TaskGetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	pageInfo.SetTotal(int(taskTotal(items, pageInfo, func() int64 {
		return model.TaskCountAllUserTask(userId, queryParams)
	})))
	pageInfo.SetItems(tasksToDto(items, false))
	common.ApiSuccess(c, pageInfo)
}

// taskTotal 计算分页总数：这一页没取满就说明已经是最后一页，直接算出来，省一次
// count(*) 查询。生产库里 tasks 和本服务不在同一台机器，每条 SQL 都是一次跨公网
// 往返（实测 300ms 起、抖动时 2s），能省则省。
func taskTotal(items []*model.Task, pageInfo *common.PageInfo, count func() int64) int64 {
	if len(items) < pageInfo.GetPageSize() {
		return int64(pageInfo.GetStartIdx() + len(items))
	}
	return count()
}

func tasksToDto(tasks []*model.Task, fillUser bool) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	if fillUser {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		for _, task := range tasks {
			userIds.Add(task.UserId)
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		result[i] = relay.TaskModel2Dto(task)
	}
	return result
}
