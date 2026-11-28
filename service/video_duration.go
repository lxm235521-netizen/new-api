package service

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const (
	videoDurationProbeTimeout   = 20 * time.Second
	videoDurationProbeChunkSize = 1 << 20  // 1 MiB per Range request
	videoDurationProbeMaxBytes  = 32 << 20 // cap for Range reads
	videoDurationProbeMaxCalls  = 64
	videoDurationProbeMaxMoov   = 32 << 20
	videoDurationProbeMaxDown   = 128 << 20
	videoDurationProbeMaxSecond = 6 * 60 * 60
)

var errRangeNotSupported = errors.New("server does not support range requests")

// GetRemoteVideoDuration probes a remote MP4/MOV URL and returns its duration
// in seconds. It uses HTTP Range requests to read only the box headers and the
// moov/mvhd payload, so long videos are not downloaded in full. Servers that do
// not support Range fall back to a bounded full download.
func GetRemoteVideoDuration(ctx context.Context, rawURL string) (float64, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return 0, errors.New("empty video url")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return 0, fmt.Errorf("unsupported video url: %s", common.MaskSensitiveInfo(rawURL))
	}

	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(
		rawURL,
		fetchSetting.EnableSSRFProtection,
		fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode,
		fetchSetting.IpFilterMode,
		fetchSetting.DomainList,
		fetchSetting.IpList,
		fetchSetting.AllowedPorts,
		fetchSetting.ApplyIPFilterForDomain,
	); err != nil {
		return 0, fmt.Errorf("video url rejected: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, videoDurationProbeTimeout)
	defer cancel()

	client := GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}

	reader, err := newHTTPRangeReadSeeker(probeCtx, client, rawURL)
	if err == nil {
		if duration, parseErr := parseMP4Duration(reader); parseErr == nil {
			return duration, nil
		}
	}

	// Range may be unsupported or the moov box may be larger than a chunk.
	// Fall back to a bounded download, which also makes the error path clearer.
	duration, downloadErr := probeRemoteVideoByDownload(probeCtx, client, rawURL)
	if downloadErr == nil {
		return duration, nil
	}
	if err != nil {
		return 0, fmt.Errorf("probe video duration: %v (fallback: %w)", err, downloadErr)
	}
	return 0, fmt.Errorf("probe video duration: %w", downloadErr)
}

// probeRemoteVideoByDownload downloads a remote video into a temporary file
// with a size cap and parses its MP4 duration. It is only used as a fallback
// when Range requests are unavailable.
func probeRemoteVideoByDownload(ctx context.Context, client *http.Client, rawURL string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if resp.ContentLength > videoDurationProbeMaxDown {
		return 0, fmt.Errorf("video is too large to probe (%d bytes)", resp.ContentLength)
	}

	file, err := os.CreateTemp("", "new-api-video-probe-*.mp4")
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()

	limited := io.LimitReader(resp.Body, videoDurationProbeMaxDown+1)
	written, err := io.Copy(file, limited)
	if err != nil {
		return 0, err
	}
	if written > videoDurationProbeMaxDown {
		return 0, fmt.Errorf("video is too large to probe (>%d bytes)", videoDurationProbeMaxDown)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	return parseMP4Duration(file)
}

// httpRangeReadSeeker implements io.ReadSeeker on top of HTTP Range requests.
type httpRangeReadSeeker struct {
	ctx    context.Context
	client *http.Client
	url    string
	size   int64

	pos        int64
	chunkStart int64
	chunk      []byte

	requests int
	fetched  int64
}

func newHTTPRangeReadSeeker(ctx context.Context, client *http.Client, rawURL string) (*httpRangeReadSeeker, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusPartialContent:
		total, err := parseContentRangeTotal(resp.Header.Get("Content-Range"))
		if err != nil {
			return nil, err
		}
		if total <= 0 {
			return nil, fmt.Errorf("invalid content-range size")
		}
		return &httpRangeReadSeeker{
			ctx:    ctx,
			client: client,
			url:    rawURL,
			size:   total,
		}, nil
	case http.StatusOK:
		if resp.ContentLength <= 0 {
			return nil, fmt.Errorf("server did not return content length")
		}
		return nil, errRangeNotSupported
	default:
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
}

func (r *httpRangeReadSeeker) Read(p []byte) (int, error) {
	if r.pos >= r.size {
		return 0, io.EOF
	}
	if r.pos < r.chunkStart || r.pos >= r.chunkStart+int64(len(r.chunk)) {
		if err := r.fetchChunk(r.pos); err != nil {
			return 0, err
		}
	}
	offset := r.pos - r.chunkStart
	n := copy(p, r.chunk[offset:])
	r.pos += int64(n)
	return n, nil
}

func (r *httpRangeReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = r.pos + offset
	case io.SeekEnd:
		next = r.size + offset
	default:
		return 0, fmt.Errorf("invalid whence %d", whence)
	}
	if next < 0 {
		return 0, fmt.Errorf("negative seek position")
	}
	r.pos = next
	return next, nil
}

func (r *httpRangeReadSeeker) fetchChunk(start int64) error {
	if start >= r.size {
		return io.EOF
	}
	if r.requests >= videoDurationProbeMaxCalls {
		return fmt.Errorf("too many range requests")
	}
	if r.fetched >= videoDurationProbeMaxBytes {
		return fmt.Errorf("range read limit exceeded")
	}

	end := start + videoDurationProbeChunkSize - 1
	if end >= r.size {
		end = r.size - 1
	}

	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	r.requests++

	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("range request returned status %d", resp.StatusCode)
	}
	rangeStart, _, _, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return err
	}
	if rangeStart != start {
		return fmt.Errorf("unexpected range start %d", rangeStart)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, videoDurationProbeChunkSize))
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("empty range response")
	}
	r.fetched += int64(len(data))
	r.chunkStart = start
	r.chunk = data
	return nil
}

func parseContentRangeTotal(value string) (int64, error) {
	_, _, total, err := parseContentRange(value)
	return total, err
}

// parseContentRange parses "bytes 0-0/12345" and returns start, end, total.
func parseContentRange(value string) (int64, int64, int64, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "bytes ") {
		return 0, 0, 0, fmt.Errorf("invalid content-range: %q", value)
	}
	value = strings.TrimSpace(strings.TrimPrefix(value, "bytes "))
	slash := strings.LastIndex(value, "/")
	if slash < 0 {
		return 0, 0, 0, fmt.Errorf("invalid content-range: %q", value)
	}
	span := value[:slash]
	totalText := strings.TrimSpace(value[slash+1:])
	dash := strings.Index(span, "-")
	if dash < 0 {
		return 0, 0, 0, fmt.Errorf("invalid content-range span: %q", span)
	}
	start, err := strconv.ParseInt(strings.TrimSpace(span[:dash]), 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	end, err := strconv.ParseInt(strings.TrimSpace(span[dash+1:]), 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	var total int64
	if totalText != "*" {
		total, err = strconv.ParseInt(totalText, 10, 64)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	return start, end, total, nil
}

// parseMP4Duration reads top-level MP4 boxes, seeks over mdat and parses
// moov/mvhd. It supports 32-bit and 64-bit box sizes and moov at either the
// beginning or the end of the file.
func parseMP4Duration(r io.ReadSeeker) (float64, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	for boxes := 0; boxes < 128; boxes++ {
		start, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}
		size, boxType, headerLen, err := readBoxHeader(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		if size == 0 {
			end, err := r.Seek(0, io.SeekEnd)
			if err != nil {
				return 0, err
			}
			size = uint64(end - start)
			if _, err := r.Seek(start+headerLen, io.SeekStart); err != nil {
				return 0, err
			}
		}
		if size < uint64(headerLen) {
			return 0, fmt.Errorf("invalid mp4 box size %d", size)
		}

		if boxType == "moov" {
			payloadLen := size - uint64(headerLen)
			if payloadLen > videoDurationProbeMaxMoov {
				return 0, fmt.Errorf("moov box is too large: %d bytes", payloadLen)
			}
			payload := make([]byte, payloadLen)
			if _, err := io.ReadFull(r, payload); err != nil {
				return 0, err
			}
			return parseMoovDuration(payload)
		}

		if _, err := r.Seek(int64(size)-headerLen, io.SeekCurrent); err != nil {
			return 0, err
		}
	}
	return 0, errors.New("moov box not found")
}

func readBoxHeader(r io.Reader) (uint64, string, int64, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, "", 0, err
	}
	size := uint64(binary.BigEndian.Uint32(header[0:4]))
	boxType := string(header[4:8])
	headerLen := int64(8)
	if size == 1 {
		var large [8]byte
		if _, err := io.ReadFull(r, large[:]); err != nil {
			return 0, "", 0, err
		}
		size = binary.BigEndian.Uint64(large[:])
		headerLen = 16
	}
	return size, boxType, headerLen, nil
}

// parseMoovDuration scans a moov payload for mvhd (and mehd for fragmented
// files) and returns the duration in seconds.
func parseMoovDuration(data []byte) (float64, error) {
	var (
		timescale   uint32
		duration    uint64
		fragmentDur uint64
		hasMVHD     bool
	)

	for offset := 0; offset+8 <= len(data); {
		size := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		boxType := string(data[offset+4 : offset+8])
		headerLen := 8
		if size == 1 {
			if offset+16 > len(data) {
				break
			}
			size = binary.BigEndian.Uint64(data[offset+8 : offset+16])
			headerLen = 16
		}
		if size == 0 {
			size = uint64(len(data) - offset)
		}
		if size < uint64(headerLen) || offset+int(size) > len(data) {
			break
		}
		payload := data[offset+headerLen : offset+int(size)]

		switch boxType {
		case "mvhd":
			ts, dur, err := parseMVHD(payload)
			if err != nil {
				return 0, err
			}
			timescale = ts
			duration = dur
			hasMVHD = true
		case "mehd":
			if len(payload) >= 8 {
				version := payload[0]
				if version == 1 && len(payload) >= 12 {
					fragmentDur = binary.BigEndian.Uint64(payload[4:12])
				} else {
					fragmentDur = uint64(binary.BigEndian.Uint32(payload[4:8]))
				}
			}
		}
		offset += int(size)
	}

	if !hasMVHD || timescale == 0 {
		return 0, errors.New("mvhd box not found or invalid")
	}
	if duration == 0 {
		duration = fragmentDur
	}
	if duration == 0 {
		return 0, errors.New("video duration is zero")
	}
	seconds := float64(duration) / float64(timescale)
	if seconds <= 0 {
		return 0, errors.New("invalid video duration")
	}
	return seconds, nil
}

func parseMVHD(payload []byte) (uint32, uint64, error) {
	if len(payload) < 20 {
		return 0, 0, errors.New("mvhd payload too short")
	}
	version := payload[0]
	if version == 1 {
		if len(payload) < 32 {
			return 0, 0, errors.New("mvhd v1 payload too short")
		}
		timescale := binary.BigEndian.Uint32(payload[20:24])
		duration := binary.BigEndian.Uint64(payload[24:32])
		return timescale, duration, nil
	}
	timescale := binary.BigEndian.Uint32(payload[12:16])
	duration := uint64(binary.BigEndian.Uint32(payload[16:20]))
	// 0xFFFFFFFF is the "unknown duration" sentinel used by fragmented MP4.
	if duration == 0xFFFFFFFF {
		duration = 0
	}
	return timescale, duration, nil
}
