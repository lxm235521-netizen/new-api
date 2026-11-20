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

package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"

	"github.com/stretchr/testify/require"
)

func TestBuildWorkbenchResolutionQuota(t *testing.T) {
	billingConfig := config.GlobalConfig.Get("billing_setting")
	require.NotNil(t, billingConfig)

	original, err := common.Marshal(billing_setting.GetResolutionPriceCopy())
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(billingConfig, map[string]string{
		"resolution_price": `{"video-model":{"720P":0.3,"1080p":0.5,"2160p":1.2}}`,
	}))
	t.Cleanup(func() {
		_ = config.UpdateConfigFromMap(billingConfig, map[string]string{
			"resolution_price": string(original),
		})
	})

	quotas := buildWorkbenchResolutionQuota(
		"video-model",
		[]string{"720p", "1080p", "4k", "1440p"},
		2,
	)
	require.Equal(t, int(0.3*common.QuotaPerUnit*2), quotas["720p"])
	require.Equal(t, int(0.5*common.QuotaPerUnit*2), quotas["1080p"])
	// 2160p 会被归一化成 4k
	require.Equal(t, int(1.2*common.QuotaPerUnit*2), quotas["4k"])
	// 未配置的档位不返回，前端回退到基础单价
	require.NotContains(t, quotas, "1440p")
	require.Len(t, quotas, 3)

	require.Nil(t, buildWorkbenchResolutionQuota("other-model", []string{"720p"}, 2))
	require.Nil(t, buildWorkbenchResolutionQuota("video-model", nil, 2))
	require.Nil(t, buildWorkbenchResolutionQuota("video-model", []string{"1440p"}, 2))
}

func TestWorkbenchModelDtoJSONResolutionQuota(t *testing.T) {
	raw, err := common.Marshal(WorkbenchModelDto{
		Model:           "video-model",
		ResolutionQuota: map[string]int{"720p": 30000, "1080p": 50000},
	})
	require.NoError(t, err)
	require.Contains(t, string(raw), `"resolution_quota"`)
	require.Contains(t, string(raw), `"720p":30000`)

	// 没有按分辨率定价时不输出该字段，前端回退到 unit_quota
	empty, err := common.Marshal(WorkbenchModelDto{Model: "video-model"})
	require.NoError(t, err)
	require.NotContains(t, string(empty), "resolution_quota")
}
