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

package service

import (
	"bytes"
	"container/list"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"

	// 解码器：按需注册，支持常见的几种出图格式
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// 缩略图尺寸限制：太小没意义，太大等于原图
const (
	thumbnailMinWidth    = 64
	thumbnailMaxWidth    = 1280
	thumbnailJPEGQuality = 85
)

// 内存缓存上限（IMAGE_THUMBNAIL_CACHE_MB，默认 256MB）。
//
// 这不是「限流」，只是给缓存定一个回收边界：命中就直接返回，超了按 LRU 丢最旧的。
// 定得大一些，常访问的图一直留在内存里，不用反复回源解码。不落盘、重启即清空，
// 多节点各存各的。
var thumbnailCacheBytes = int64(common.GetEnvOrDefault("IMAGE_THUMBNAIL_CACHE_MB", 256)) << 20

// NormalizeThumbnailWidth 校验前端传来的宽度；返回 0 表示不生成缩略图（按原图返回）
func NormalizeThumbnailWidth(width int) int {
	if width < thumbnailMinWidth || width > thumbnailMaxWidth {
		return 0
	}
	return width
}

// BuildImageThumbnail 把原图等比缩到指定宽度。
//
// 卡片上只显示一两百像素宽的预览，原图却有 2~3MB：不缩放的话首屏要等好几秒白块。
// PNG 仍出 PNG（可能是带透明的素材/二维码，转 JPEG 会变黑底），其它格式出 JPEG。
// 缩放用 CatmullRom，缩小后的清晰度比最近邻好很多。
func BuildImageThumbnail(raw []byte, contentType string, width int) ([]byte, string, error) {
	if len(raw) == 0 {
		return nil, "", fmt.Errorf("empty image data")
	}

	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", fmt.Errorf("decode image failed: %w", err)
	}

	srcBounds := src.Bounds()
	srcWidth, srcHeight := srcBounds.Dx(), srcBounds.Dy()
	if srcWidth <= 0 || srcHeight <= 0 {
		return nil, "", fmt.Errorf("invalid image size %dx%d", srcWidth, srcHeight)
	}

	if srcWidth <= width {
		// 原图本来就够小，直接用
		return raw, contentType, nil
	}

	targetHeight := int(float64(srcHeight) * float64(width) / float64(srcWidth))
	if targetHeight < 1 {
		targetHeight = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, targetHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, srcBounds, draw.Src, nil)

	// 带透明的素材必须留 PNG（转 JPEG 透明处会变黑底）；不透明的照片出 JPEG ——
	// 同样尺寸下 PNG 能到 500KB，JPEG 只要几十 KB。
	if isPNG(contentType) && hasTransparency(dst) {
		var pngBuf bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&pngBuf, dst); err != nil {
			return nil, "", fmt.Errorf("encode png failed: %w", err)
		}
		return pngBuf.Bytes(), "image/png", nil
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: thumbnailJPEGQuality}); err != nil {
		return nil, "", fmt.Errorf("encode jpeg failed: %w", err)
	}
	return buf.Bytes(), "image/jpeg", nil
}

// hasTransparency 扫一遍 alpha 通道，判断缩小后的图是否需要保留透明
func hasTransparency(img *image.NRGBA) bool {
	if img == nil {
		return false
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	for y := 0; y < bounds.Dy(); y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+width*4]
		for i := 3; i < len(row); i += 4 {
			if row[i] != 0xff {
				return true
			}
		}
	}
	return false
}

func isPNG(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "png")
}

// ---------------------------------------------------------------- 内存缓存

type thumbnailEntry struct {
	key         string
	body        []byte
	contentType string
}

var thumbnailCache = struct {
	sync.Mutex
	items map[string]*list.Element
	order *list.List
	bytes int64
}{
	items: make(map[string]*list.Element),
	order: list.New(),
}

// GetImageThumbnail 命中则返回缩略图，未命中返回 false
func GetImageThumbnail(key string) ([]byte, string, bool) {
	thumbnailCache.Lock()
	defer thumbnailCache.Unlock()

	element, ok := thumbnailCache.items[key]
	if !ok {
		return nil, "", false
	}
	thumbnailCache.order.MoveToFront(element)
	entry := element.Value.(*thumbnailEntry)
	return entry.body, entry.contentType, true
}

// PutImageThumbnail 写入缓存并按需淘汰（LRU，超限就从最旧的开始丢）
func PutImageThumbnail(key string, body []byte, contentType string) {
	if key == "" || len(body) == 0 {
		return
	}
	if int64(len(body)) > thumbnailCacheBytes {
		return
	}

	thumbnailCache.Lock()
	defer thumbnailCache.Unlock()

	if element, ok := thumbnailCache.items[key]; ok {
		entry := element.Value.(*thumbnailEntry)
		thumbnailCache.bytes += int64(len(body) - len(entry.body))
		entry.body = body
		entry.contentType = contentType
		thumbnailCache.order.MoveToFront(element)
	} else {
		element := thumbnailCache.order.PushFront(&thumbnailEntry{
			key:         key,
			body:        body,
			contentType: contentType,
		})
		thumbnailCache.items[key] = element
		thumbnailCache.bytes += int64(len(body))
	}

	for thumbnailCache.bytes > thumbnailCacheBytes {
		oldest := thumbnailCache.order.Back()
		if oldest == nil {
			break
		}
		thumbnailCache.order.Remove(oldest)
		entry := oldest.Value.(*thumbnailEntry)
		delete(thumbnailCache.items, entry.key)
		thumbnailCache.bytes -= int64(len(entry.body))
	}
}

// ThumbnailCacheStats 供排查用（当前缓存条数与占用）
func ThumbnailCacheStats() (int, int64) {
	thumbnailCache.Lock()
	defer thumbnailCache.Unlock()
	return len(thumbnailCache.items), thumbnailCache.bytes
}
