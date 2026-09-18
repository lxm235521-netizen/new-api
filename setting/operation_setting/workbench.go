package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const workbenchConfigName = "workbench_setting"

// 模型分组
const (
	WorkbenchGroupVideo = "video"
	WorkbenchGroupImage = "image"
)

// 计费方式
const (
	WorkbenchBillingPerSecond = "per_second"
	WorkbenchBillingPerCall   = "per_call"
)

// 时长模式
const (
	// WorkbenchDurationList 从固定候选值里选（例如只支持 5 / 10 秒）
	WorkbenchDurationList = "list"
	// WorkbenchDurationRange 连续区间（例如 1~30 秒）
	WorkbenchDurationRange = "range"
)

// WorkbenchDuration 描述一个模型支持的时长。
//
// 空配置（Mode 为空且 Values 为空）表示该模型不需要时长项，
// 前端会直接隐藏该控件 —— 图片类模型就是这样。
type WorkbenchDuration struct {
	Mode   string `json:"mode"`   // "list" | "range"
	Values []int  `json:"values"` // list 模式：候选秒数
	Min    int    `json:"min"`    // range 模式：最小秒数
	Max    int    `json:"max"`    // range 模式：最大秒数
	Step   int    `json:"step"`   // range 模式：步长
}

// WorkbenchReference 一类参考素材的开关与数量上限。
type WorkbenchReference struct {
	Enabled bool `json:"enabled"`
	Max     int  `json:"max"`
}

// WorkbenchReferences 三类参考素材。不同模型支持的种类和数量都不同
// （例如只支持图片、或者图片最多 7 张而另一模型 9 张）。
type WorkbenchReferences struct {
	Image WorkbenchReference `json:"image"`
	Audio WorkbenchReference `json:"audio"`
	Video WorkbenchReference `json:"video"`
}

// WorkbenchPromptOptimize 提示词优化配置。
//
// Model 是用于改写的推理模型名（走 /pg/chat/completions，即由本系统路由，
// 不在前端暴露任何上游 Key）；SystemPrompt 留空则用内置默认指令。
type WorkbenchPromptOptimize struct {
	Enabled      bool   `json:"enabled"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
}

// WorkbenchModel 是工作台展示的一个模型及其完整参数能力。
type WorkbenchModel struct {
	Model          string                  `json:"model"`
	Group          string                  `json:"group"`
	Billing        string                  `json:"billing"`
	Duration       WorkbenchDuration       `json:"duration"`
	Resolutions    []string                `json:"resolutions"`
	AspectRatios   []string                `json:"aspect_ratios"`
	References     WorkbenchReferences     `json:"references"`
	PromptOptimize WorkbenchPromptOptimize `json:"prompt_optimize"`
}

// WorkbenchSetting 保存管理员为工作台挑选的模型目录。
// 空列表表示前端回退到内置的默认目录。
type WorkbenchSetting struct {
	Models []WorkbenchModel `json:"models"`
	// AsyncImagePerUser 异步图片生成时「单个用户同时可跑几张」。
	// 默认 2；设为 0 表示不限（此时并发完全交给上游排队）。
	// 它挡的是「一个用户开一堆标签页把上游占满」，不影响外部 API 接入。
	AsyncImagePerUser int `json:"async_image_per_user"`
}

var workbenchSetting = WorkbenchSetting{
	Models:            []WorkbenchModel{},
	AsyncImagePerUser: 2,
}

func init() {
	config.GlobalConfig.Register(workbenchConfigName, &workbenchSetting)
}

// GetWorkbenchSetting 返回工作台配置（指针，随配置热更新）。
func GetWorkbenchSetting() *WorkbenchSetting {
	return &workbenchSetting
}

func trimStringList(values []string) []string {
	if values == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

// normalizeDuration 校验时长配置。
//
// 关键点：nil 与空切片含义不同 ——
//   - nil 表示管理员没配过（老数据），按分组补默认值
//   - 空切片表示管理员明确清空，即"该模型不需要时长项"，此时保持为空
func normalizeDuration(duration WorkbenchDuration, group string) WorkbenchDuration {
	empty := duration.Mode == "" && duration.Values == nil &&
		duration.Min == 0 && duration.Max == 0

	if empty {
		if group == WorkbenchGroupImage {
			return WorkbenchDuration{}
		}
		return WorkbenchDuration{Mode: WorkbenchDurationList, Values: []int{5, 10}}
	}

	if duration.Mode == WorkbenchDurationRange {
		if duration.Min < 0 {
			duration.Min = 0
		}
		if duration.Max < duration.Min {
			duration.Max = duration.Min
		}
		if duration.Step <= 0 {
			duration.Step = 1
		}
		return WorkbenchDuration{
			Mode: WorkbenchDurationRange,
			Min:  duration.Min,
			Max:  duration.Max,
			Step: duration.Step,
		}
	}

	values := duration.Values
	if values == nil {
		values = []int{}
	}
	seen := make(map[int]struct{}, len(values))
	cleaned := make([]int, 0, len(values))
	for _, value := range values {
		if value < 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}

	// 既不是 range 又没有候选值 -> 视为不需要时长项
	if len(cleaned) == 0 {
		return WorkbenchDuration{}
	}
	return WorkbenchDuration{Mode: WorkbenchDurationList, Values: cleaned}
}

func normalizeReference(reference WorkbenchReference) WorkbenchReference {
	if reference.Max < 0 {
		reference.Max = 0
	}
	if !reference.Enabled {
		reference.Max = 0
	}
	if reference.Enabled && reference.Max == 0 {
		reference.Max = 1
	}
	return reference
}

// NormalizeWorkbenchSetting 清理脏数据并补齐默认值。
//
// 尽量"宽容"：非法分组/计费方式回落到安全值，重复项去重，
// 空模型名丢弃；这样管理员写入脏数据也不会让前端崩掉。
func NormalizeWorkbenchSetting() {
	// 异步图片并发：负数归零（0 = 不限）；没配过（老数据 0）给默认 2
	if workbenchSetting.AsyncImagePerUser < 0 {
		workbenchSetting.AsyncImagePerUser = 0
	}

	if len(workbenchSetting.Models) == 0 {
		workbenchSetting.Models = []WorkbenchModel{}
		return
	}

	seen := make(map[string]struct{}, len(workbenchSetting.Models))
	normalized := make([]WorkbenchModel, 0, len(workbenchSetting.Models))

	for _, item := range workbenchSetting.Models {
		modelName := strings.TrimSpace(item.Model)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}

		group := strings.TrimSpace(item.Group)
		if group != WorkbenchGroupImage {
			group = WorkbenchGroupVideo
		}

		billing := strings.TrimSpace(item.Billing)
		if billing != WorkbenchBillingPerSecond {
			billing = WorkbenchBillingPerCall
		}

		resolutions := trimStringList(item.Resolutions)
		if resolutions == nil && group == WorkbenchGroupVideo {
			// 老数据没有该字段：视频给一组常见分辨率；图片保持为空（前端隐藏）
			resolutions = []string{"480p", "720p", "1080p"}
		}
		if resolutions == nil {
			resolutions = []string{}
		}

		aspectRatios := trimStringList(item.AspectRatios)
		if aspectRatios == nil {
			aspectRatios = []string{"16:9", "9:16", "1:1"}
		}
		if len(aspectRatios) == 0 {
			aspectRatios = []string{"16:9"}
		}

		references := item.References
		if !references.Image.Enabled && !references.Audio.Enabled &&
			!references.Video.Enabled && references.Image.Max == 0 {
			// 老数据没有该字段：默认开启参考图
			references.Image = WorkbenchReference{Enabled: true, Max: 4}
		}
		references.Image = normalizeReference(references.Image)
		references.Audio = normalizeReference(references.Audio)
		references.Video = normalizeReference(references.Video)

		promptOptimize := item.PromptOptimize
		promptOptimize.Model = strings.TrimSpace(promptOptimize.Model)
		promptOptimize.SystemPrompt = strings.TrimSpace(promptOptimize.SystemPrompt)
		// 刻意不因为「没选模型」就把 Enabled 改回 false：那样数据库里的
		// enabled=true 与内存里的 false 会长期不一致，管理员重新打开配置
		// 看到的是「已启用」，却始终等不到按钮。
		// 前端用 `enabled && model` 双重判断决定是否渲染按钮，
		// 配置页则在保存时就拦住「开了优化但没填模型」这种组合。

		normalized = append(normalized, WorkbenchModel{
			Model:          modelName,
			Group:          group,
			Billing:        billing,
			Duration:       normalizeDuration(item.Duration, group),
			Resolutions:    resolutions,
			AspectRatios:   aspectRatios,
			References:     references,
			PromptOptimize: promptOptimize,
		})
	}

	workbenchSetting.Models = normalized
}
