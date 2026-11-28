package billing_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/require"
)

func TestInputVideoDurationSettingUpdate(t *testing.T) {
	oldUnits := billingSetting.PriceUnit
	oldFlags := billingSetting.InputVideoDuration
	defer func() {
		billingSetting.PriceUnit = oldUnits
		billingSetting.InputVideoDuration = oldFlags
	}()

	err := config.UpdateConfigFromMap(&billingSetting, map[string]string{
		PriceUnitField:          `{"video_remote_srt":"per_second"}`,
		InputVideoDurationField: `{"video_remote_srt":true}`,
	})
	require.NoError(t, err)

	require.True(t, IsPerSecondPriced("video_remote_srt"))
	require.True(t, IsInputVideoDurationBilling("video_remote_srt"))
	require.False(t, IsInputVideoDurationBilling("other-model"))

	syncData := GetPricingSyncData(nil)
	flags, ok := syncData[InputVideoDurationField].(map[string]bool)
	require.True(t, ok)
	require.True(t, flags["video_remote_srt"])
}

func TestInputVideoDurationSettingClear(t *testing.T) {
	oldFlags := billingSetting.InputVideoDuration
	defer func() { billingSetting.InputVideoDuration = oldFlags }()

	require.NoError(t, config.UpdateConfigFromMap(&billingSetting, map[string]string{
		InputVideoDurationField: `{"video_remote_srt":true}`,
	}))
	require.True(t, IsInputVideoDurationBilling("video_remote_srt"))

	require.NoError(t, config.UpdateConfigFromMap(&billingSetting, map[string]string{
		InputVideoDurationField: `{}`,
	}))
	require.False(t, IsInputVideoDurationBilling("video_remote_srt"))
}
