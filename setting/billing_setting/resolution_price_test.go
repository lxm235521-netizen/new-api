package billing_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeResolutionKey(t *testing.T) {
	cases := map[string]string{
		"480p":      "480p",
		"480P":      "480p",
		"720p":      "720p",
		"720P":      "720p",
		"768p":      "768p",
		"768P":      "768p",
		"1080p":     "1080p",
		"1080P":     "1080p",
		"2160p":     "4k",
		"4K":        "4k",
		"uhd":       "4k",
		"1280x720":  "720p",
		"1366x768":  "768p",
		"1920x1080": "1080p",
		"1792x1024": "1080p",
		"854x480":   "480p",
		"3840x2160": "4k",
		"832*480":   "480p",
		"1920*1080": "1080p",
		"1080*1920": "1080p",
		"720×1280":  "720p",
		"720":       "720p",
	}
	for input, want := range cases {
		require.Equal(t, want, NormalizeResolutionKey(input), "input=%s", input)
	}
}

func TestGetResolutionPriceMatchesNormalizedKeys(t *testing.T) {
	original := billingSetting.ResolutionPrice
	billingSetting.ResolutionPrice = map[string]map[string]float64{
		"video-model": {
			"480p":  0.1,
			"720P":  0.2,
			"768p":  0.25,
			"1080p": 0.3,
			"2160p": 0.6,
		},
	}
	t.Cleanup(func() {
		billingSetting.ResolutionPrice = original
	})

	price, ok := GetResolutionPrice("video-model", "720p")
	require.True(t, ok)
	require.Equal(t, 0.2, price)

	price, ok = GetResolutionPrice("video-model", "768P")
	require.True(t, ok)
	require.Equal(t, 0.25, price)

	price, ok = GetResolutionPrice("video-model", "1920x1080")
	require.True(t, ok)
	require.Equal(t, 0.3, price)

	price, ok = GetResolutionPrice("video-model", "4k")
	require.True(t, ok)
	require.Equal(t, 0.6, price)

	_, ok = GetResolutionPrice("video-model", "1440p")
	require.False(t, ok)

	_, ok = GetResolutionPrice("other-model", "720p")
	require.False(t, ok)
}

func TestGetResolutionPricesReturnsNormalizedCopy(t *testing.T) {
	original := billingSetting.ResolutionPrice
	billingSetting.ResolutionPrice = map[string]map[string]float64{
		"video-model": {
			"720P":  0.2,
			"1080p": 0.3,
			"2160p": 0.6,
			"bad":   -1,
		},
		"empty-model": {},
	}
	t.Cleanup(func() {
		billingSetting.ResolutionPrice = original
	})

	prices := GetResolutionPrices("video-model")
	require.Equal(t, map[string]float64{
		"720p":  0.2,
		"1080p": 0.3,
		"4k":    0.6,
	}, prices)

	// The returned map must be a copy: mutating it cannot affect the setting.
	prices["720p"] = 99
	require.Equal(t, 0.2, billingSetting.ResolutionPrice["video-model"]["720P"])

	require.Nil(t, GetResolutionPrices("empty-model"))
	require.Nil(t, GetResolutionPrices("missing-model"))
}

func TestGetResolutionPricesPrefersCanonicalKey(t *testing.T) {
	original := billingSetting.ResolutionPrice
	billingSetting.ResolutionPrice = map[string]map[string]float64{
		"video-model": {
			"2160P": 1,
			"2160p": 3,
			"4k":    2,
		},
	}
	t.Cleanup(func() {
		billingSetting.ResolutionPrice = original
	})

	// 三种写法都归一化到 4k，规范写法 "4k" 优先，且多次读取结果稳定
	for i := 0; i < 20; i++ {
		require.Equal(t, 2.0, GetResolutionPrices("video-model")["4k"])
		price, ok := GetResolutionPrice("video-model", "2160p")
		require.True(t, ok)
		require.Equal(t, 2.0, price)
	}
}
