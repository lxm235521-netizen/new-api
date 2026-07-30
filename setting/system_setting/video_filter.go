package system_setting

import (
	"slices"

	"github.com/QuantumNous/new-api/setting/config"
)

type VideoFilterSetting struct {
	HiddenFieldsModels []string `json:"hidden_fields_models"`
}

var defaultVideoFilterSetting = VideoFilterSetting{
	HiddenFieldsModels: []string{},
}

func init() {
	config.GlobalConfig.Register("video_filter_setting", &defaultVideoFilterSetting)
}

func GetVideoFilterSetting() *VideoFilterSetting {
	return &defaultVideoFilterSetting
}

func ShouldStripTaskData(modelName string) bool {
	if modelName == "" {
		return false
	}
	return slices.Contains(defaultVideoFilterSetting.HiddenFieldsModels, modelName)
}
