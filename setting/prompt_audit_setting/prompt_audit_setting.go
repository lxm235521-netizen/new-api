package prompt_audit_setting

import "github.com/QuantumNous/new-api/setting/config"

// PromptAuditSetting 定向请求审计配置。
//
// 该配置项通过 config.GlobalConfig 注册后会以
// prompt_audit_setting.<json key> 的形式落在 options 表，
// 后台（系统设置，仅 root 可见）保存后由 updateOptionMap 直接刷新内存，
// 无需重启即可生效。
type PromptAuditSetting struct {
	Enabled    bool   `json:"enabled"`     // 是否启用定向请求审计
	Users      string `json:"users"`       // 被监控用户，逗号分隔（用户名或用户 id），留空表示所有用户
	Models     string `json:"models"`      // 被监控模型，逗号分隔，支持 * 通配，不带 * 时按前缀匹配，留空表示所有模型
	MaxBytes   int    `json:"max_bytes"`   // 单条正文最大字节数，0 或负数表示不截断（完整输入）
	SkipOutput bool   `json:"skip_output"` // 只记录输入，不记录输出与流式分片
}

// 默认配置：默认关闭，未命中过滤条件时不产生任何 IO。
var promptAuditSetting = PromptAuditSetting{
	Enabled:    false,
	Users:      "",
	Models:     "",
	MaxBytes:   0,
	SkipOutput: false,
}

func init() {
	config.GlobalConfig.Register("prompt_audit_setting", &promptAuditSetting)
}

// GetPromptAuditSetting 获取请求审计配置（返回的是注册对象本身，调用方只读）。
func GetPromptAuditSetting() *PromptAuditSetting {
	return &promptAuditSetting
}
