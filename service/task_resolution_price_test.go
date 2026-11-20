package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveTaskResolutionPrefersResolutionField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	req := relaycommon.TaskSubmitReq{
		Resolution: "768P",
		Size:       "1920x1080",
		Metadata: map[string]interface{}{
			"resolution": "720p",
		},
	}

	got, ok := ResolveTaskResolution(c, req)
	require.True(t, ok)
	require.Equal(t, "768p", got)
}

func TestResolveTaskResolutionFallsBackToMetadataAndSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	got, ok := ResolveTaskResolution(c, relaycommon.TaskSubmitReq{
		Metadata: map[string]interface{}{"resolution": "720p"},
		Size:     "1920x1080",
	})
	require.True(t, ok)
	require.Equal(t, "720p", got)

	got, ok = ResolveTaskResolution(c, relaycommon.TaskSubmitReq{Size: "1366x768"})
	require.True(t, ok)
	require.Equal(t, "768p", got)

	got, ok = ResolveTaskResolution(c, relaycommon.TaskSubmitReq{Size: "1920x1080"})
	require.True(t, ok)
	require.Equal(t, "1080p", got)
}

func TestResolveTaskResolutionUsesRemixContextFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ContextKeyBillingResolution, "1080p")

	got, ok := ResolveTaskResolution(c, relaycommon.TaskSubmitReq{})
	require.True(t, ok)
	require.Equal(t, "1080p", got)

	// An explicit metadata.resolution on the remix request overrides the
	// original task's tier, while top-level resolution still has top priority.
	got, ok = ResolveTaskResolution(c, relaycommon.TaskSubmitReq{
		Metadata: map[string]interface{}{"resolution": "720p"},
	})
	require.True(t, ok)
	require.Equal(t, "720p", got)
}

func TestApplyTaskResolutionPriceOverridesBaseAndKeepsSeconds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:      "video-model",
		Resolution: "768p",
		Duration:   8,
	})

	billingConfig := config.GlobalConfig.Get("billing_setting")
	require.NotNil(t, billingConfig)
	originalUnits, _ := common.Marshal(billing_setting.GetPriceUnitCopy())
	originalPrices, _ := common.Marshal(billing_setting.GetResolutionPriceCopy())
	require.NoError(t, config.UpdateConfigFromMap(billingConfig, map[string]string{
		"price_unit":       `{"video-model":"per_second"}`,
		"resolution_price": `{"video-model":{"480p":0.1,"720p":0.2,"768p":0.25,"1080p":0.3}}`,
	}))
	t.Cleanup(func() {
		_ = config.UpdateConfigFromMap(billingConfig, map[string]string{
			"price_unit":       string(originalUnits),
			"resolution_price": string(originalPrices),
		})
	})

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
		PriceData: types.PriceData{
			ModelPrice:     1,
			UsePrice:       true,
			Quota:          100,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
			OtherRatios:    map[string]float64{"resolution": 1.5},
		},
	}
	info.OriginModelName = "video-model"

	ok := ApplyTaskResolutionPrice(c, info)
	require.True(t, ok)
	require.Equal(t, 0.25, info.PriceData.ModelPrice)
	require.Equal(t, int(0.25*common.QuotaPerUnit*2), info.PriceData.Quota)
	require.NotContains(t, info.PriceData.OtherRatios, "resolution")
	require.Equal(t, 8.0, info.PriceData.OtherRatios["seconds"])
	require.Equal(t, "768p", info.PriceData.Resolution)
}
