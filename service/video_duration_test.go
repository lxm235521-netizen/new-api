package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func mp4Box(boxType string, payload []byte) []byte {
	result := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(result[0:4], uint32(len(result)))
	copy(result[4:8], boxType)
	copy(result[8:], payload)
	return result
}

func mvhdV0(timescale, duration uint32) []byte {
	payload := make([]byte, 20)
	// version(1) + flags(3) + creation(4) + modification(4)
	binary.BigEndian.PutUint32(payload[12:16], timescale)
	binary.BigEndian.PutUint32(payload[16:20], duration)
	return mp4Box("mvhd", payload)
}

func mvhdV1(timescale uint32, duration uint64) []byte {
	payload := make([]byte, 32)
	payload[0] = 1
	binary.BigEndian.PutUint32(payload[20:24], timescale)
	binary.BigEndian.PutUint64(payload[24:32], duration)
	return mp4Box("mvhd", payload)
}

func TestParseMP4Duration(t *testing.T) {
	ftyp := mp4Box("ftyp", []byte("isom\x00\x00\x02\x00isomiso2"))
	mdat := mp4Box("mdat", bytes.Repeat([]byte{0x01}, 128))
	moov := mp4Box("moov", mvhdV0(1000, 15080))

	t.Run("moov before mdat", func(t *testing.T) {
		data := append(append(append([]byte{}, ftyp...), moov...), mdat...)
		duration, err := parseMP4Duration(bytes.NewReader(data))
		require.NoError(t, err)
		require.InDelta(t, 15.08, duration, 0.0001)
	})

	t.Run("moov after mdat", func(t *testing.T) {
		data := append(append(append([]byte{}, ftyp...), mdat...), moov...)
		duration, err := parseMP4Duration(bytes.NewReader(data))
		require.NoError(t, err)
		require.InDelta(t, 15.08, duration, 0.0001)
	})

	t.Run("version 1 mvhd", func(t *testing.T) {
		moovV1 := mp4Box("moov", mvhdV1(90000, 90*90000))
		data := append(append([]byte{}, ftyp...), moovV1...)
		duration, err := parseMP4Duration(bytes.NewReader(data))
		require.NoError(t, err)
		require.InDelta(t, 90.0, duration, 0.0001)
	})

	t.Run("large mdat size", func(t *testing.T) {
		largeMdat := make([]byte, 16+64)
		binary.BigEndian.PutUint32(largeMdat[0:4], 1)
		copy(largeMdat[4:8], "mdat")
		binary.BigEndian.PutUint64(largeMdat[8:16], uint64(len(largeMdat)))
		data := append(append(append([]byte{}, ftyp...), largeMdat...), moov...)
		duration, err := parseMP4Duration(bytes.NewReader(data))
		require.NoError(t, err)
		require.InDelta(t, 15.08, duration, 0.0001)
	})

	t.Run("missing moov", func(t *testing.T) {
		data := append(append([]byte{}, ftyp...), mdat...)
		_, err := parseMP4Duration(bytes.NewReader(data))
		require.Error(t, err)
	})
}

func TestParseContentRange(t *testing.T) {
	start, end, total, err := parseContentRange("bytes 0-0/12345")
	require.NoError(t, err)
	require.Equal(t, int64(0), start)
	require.Equal(t, int64(0), end)
	require.Equal(t, int64(12345), total)

	start, end, total, err = parseContentRange("bytes 100-199/*")
	require.NoError(t, err)
	require.Equal(t, int64(100), start)
	require.Equal(t, int64(199), end)
	require.Equal(t, int64(0), total)

	_, _, _, err = parseContentRange("nonsense")
	require.Error(t, err)
}

func TestHTTPRangeReadSeeker(t *testing.T) {
	body := append(append(mp4Box("ftyp", []byte("isom\x00\x00\x02\x00")), mp4Box("mdat", bytes.Repeat([]byte{0x02}, 70000))...), mp4Box("moov", mvhdV0(1000, 15080))...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			_, _ = w.Write(body)
			return
		}
		rangeHeader = strings.TrimPrefix(rangeHeader, "bytes=")
		parts := strings.SplitN(rangeHeader, "-", 2)
		start, _ := strconv.Atoi(parts[0])
		end, _ := strconv.Atoi(parts[1])
		if end >= len(body) {
			end = len(body) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[start : end+1])
	}))
	defer server.Close()

	reader, err := newHTTPRangeReadSeeker(context.Background(), server.Client(), server.URL)
	require.NoError(t, err)
	duration, err := parseMP4Duration(reader)
	require.NoError(t, err)
	require.InDelta(t, 15.08, duration, 0.0001)
}

func TestHTTPRangeReadSeekerUnsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "16")
		_, _ = w.Write(make([]byte, 16))
	}))
	defer server.Close()

	_, err := newHTTPRangeReadSeeker(context.Background(), server.Client(), server.URL)
	require.ErrorIs(t, err, errRangeNotSupported)
}

func TestCollectTaskVideoURLs(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Videos: []string{
			"https://example.com/a.mp4",
			" https://example.com/a.mp4 ",
			"ftp://example.com/bad.mp4",
		},
		Metadata: map[string]interface{}{
			"videos": []interface{}{
				"https://example.com/b.mp4",
				"https://example.com/a.mp4",
			},
			"content": []interface{}{
				map[string]interface{}{
					"video_url": map[string]interface{}{"url": "https://example.com/c.mp4"},
				},
			},
		},
	}
	urls := collectTaskVideoURLs(req)
	require.Equal(t, []string{
		"https://example.com/a.mp4",
		"https://example.com/b.mp4",
		"https://example.com/c.mp4",
	}, urls)
}

// TestRemoteVideoDurationIntegration is opt-in: set VIDEO_DURATION_TEST_URL to
// a public MP4 URL to verify Range-based probing against a real CDN.
func TestRemoteVideoDurationIntegration(t *testing.T) {
	videoURL := os.Getenv("VIDEO_DURATION_TEST_URL")
	if videoURL == "" {
		t.Skip("VIDEO_DURATION_TEST_URL not set")
	}
	duration, err := GetRemoteVideoDuration(context.Background(), videoURL)
	require.NoError(t, err)
	require.Greater(t, duration, 0.0)
	t.Logf("duration=%f", duration)
}

func TestApplyInputVideoDurationBilling(t *testing.T) {
	cfg, ok := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	require.True(t, ok)

	oldUnits := cfg.PriceUnit
	oldFlags := cfg.InputVideoDuration
	cfg.PriceUnit = map[string]string{"video-model": billing_setting.PriceUnitPerSecond}
	cfg.InputVideoDuration = map[string]bool{"video-model": true}
	defer func() {
		cfg.PriceUnit = oldUnits
		cfg.InputVideoDuration = oldFlags
	}()

	oldProbe := probeRemoteVideoDuration
	probeRemoteVideoDuration = func(_ context.Context, videoURL string) (float64, error) {
		if strings.Contains(videoURL, "short") {
			return 0.2, nil
		}
		return 15.08, nil
	}
	defer func() { probeRemoteVideoDuration = oldProbe }()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{}`))
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Videos:  []string{"https://example.com/long.mp4", "https://example.com/short.mp4"},
		Seconds: "4",
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "video-model",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "video-model"},
	}
	require.NoError(t, ApplyInputVideoDurationBilling(c, info, "video-model"))

	req, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	require.Equal(t, 16, req.Duration)
	require.Empty(t, req.Seconds)
}
