package common

import (
	"bytes"
	"encoding/json"
	"io"
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
