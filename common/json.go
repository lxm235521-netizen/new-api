package common

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func UnmarshalJsonStr(data string, v any) error {
	return json.Unmarshal(StringToByteSlice(data), v)
}

func DecodeJson(reader io.Reader, v any) error {
	return json.NewDecoder(reader).Decode(v)
}

func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func GetJsonType(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "unknown"
	}
	firstChar := trimmed[0]
	switch firstChar {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// StripTaskSensitiveKeys removes video_url and download_url keys from all
// nesting levels of the given JSON. Returns the original data on any error.
func StripTaskSensitiveKeys(data json.RawMessage) json.RawMessage {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return data
	}
	walkAndStripSensitive(v)
	result, err := json.Marshal(v)
	if err != nil {
		return data
	}
	return result
}

func walkAndStripSensitive(v any) {
	switch val := v.(type) {
	case map[string]any:
		delete(val, "video_url")
		delete(val, "download_url")
		for _, child := range val {
			walkAndStripSensitive(child)
		}
	case []any:
		for i := range val {
			walkAndStripSensitive(val[i])
		}
	}
}

// ReplaceTaskVideoURLs replaces video result URL fields with proxyURL while
// leaving unrelated request/input URLs untouched as much as possible.
func ReplaceTaskVideoURLs(data json.RawMessage, proxyURL string) json.RawMessage {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return data
	}
	var v any
	if err := Unmarshal(data, &v); err != nil {
		return data
	}
	walkAndReplaceTaskVideoURLs(v, proxyURL, "")
	result, err := Marshal(v)
	if err != nil {
		return data
	}
	return result
}

func walkAndReplaceTaskVideoURLs(v any, proxyURL string, parentKey string) {
	switch val := v.(type) {
	case map[string]any:
		for key, child := range val {
			lowerKey := strings.ToLower(key)
			if lowerKey == "b64_json" {
				// 上游把图直接塞成 base64：一条就 1~3MB，列表接口一次 9 条能到 8MB。
				// 这里的值全部清空（要图请走代理地址）。
				val[key] = ""
				continue
			}
			if isReplaceableTaskVideoURLField(lowerKey, parentKey, val) && isReplaceableMediaValue(child) {
				val[key] = proxyURL
				continue
			}
			walkAndReplaceTaskVideoURLs(child, proxyURL, lowerKey)
		}
	case []any:
		for i := range val {
			walkAndReplaceTaskVideoURLs(val[i], proxyURL, parentKey)
		}
	}
}

func isReplaceableTaskVideoURLField(key string, parentKey string, container map[string]any) bool {
	switch key {
	case "video_url", "download_url":
		return true
	case "url", "image_url":
		// 图片网关常见返回是 {"model":"gpt-image-2","url":"data:image/png;base64,..."}：
		// 没有 video 字样、父级也不叫 video，只能按「值本身是不是媒体地址」判断
		if isDataURLValue(container[key]) {
			return true
		}
		return isVideoURLContainer(parentKey, container)
	case "object":
		if !isReplaceableMediaValue(container[key]) {
			return false
		}
		_, hasStatus := container["status"]
		_, hasTaskID := container["task_id"]
		_, hasProgress := container["progress"]
		return hasStatus || hasTaskID || hasProgress
	default:
		return false
	}
}

func isVideoURLContainer(parentKey string, container map[string]any) bool {
	switch parentKey {
	case "video", "videos", "creation", "creations", "metadata", "output", "result", "task_result":
		return true
	}
	if _, ok := container["video_url"]; ok {
		return true
	}
	if _, ok := container["download_url"]; ok {
		return true
	}
	if _, ok := container["video"]; ok {
		return true
	}
	if object, ok := container["object"].(string); ok && strings.Contains(strings.ToLower(object), "video") {
		return true
	}
	if model, ok := container["model"].(string); ok && strings.Contains(strings.ToLower(model), "video") {
		return true
	}
	return false
}

func isRemoteURLValue(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	s = strings.TrimSpace(strings.ToLower(s))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// isDataURLValue 判断是不是 data:image/...;base64,xxxx 这种内联图片。
// 这类值动辄 1~3MB，绝不能原样回给前端。
func isDataURLValue(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(s)), "data:")
}

// isReplaceableMediaValue 媒体地址：http(s) 或内联 data URL
func isReplaceableMediaValue(v any) bool {
	return isRemoteURLValue(v) || isDataURLValue(v)
}

// JsonRawMessageToString returns JSON strings as their decoded value and other JSON values as raw text.
func JsonRawMessageToString(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] != '"' {
		return string(trimmed)
	}
	var value string
	if err := Unmarshal(trimmed, &value); err != nil {
		return string(trimmed)
	}
	return value
}
