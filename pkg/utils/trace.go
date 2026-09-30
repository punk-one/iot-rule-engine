package utils

import (
	"encoding/hex"
	"strings"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

// NormalizeTraceID 归一化 trace 标识。
// 如果传入的是 W3C traceparent，则提取其中的 trace-id。
func NormalizeTraceID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if traceID := parseTraceParent(raw); traceID != "" {
		return traceID
	}

	return raw
}

// FirstNonEmptyTraceID 返回第一个有效的 trace 标识。
func FirstNonEmptyTraceID(values ...string) string {
	for _, value := range values {
		if normalized := NormalizeTraceID(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

// MetadataTraceID 从常见 metadata 键中提取 trace 标识。
func MetadataTraceID(metadata map[string]string) string {
	if metadata == nil {
		return ""
	}

	return FirstNonEmptyTraceID(
		metadata[sdk.EventMetadataTraceIDKey],
		metadata["x-trace-id"],
		metadata["x-request-id"],
		metadata["traceparent"],
	)
}

// EnsureEventTraceID 确保事件具备 trace 标识。
// 优先保留事件现有 trace，其次使用候选值，再次尝试从 metadata 中提取，最后自动生成。
func EnsureEventTraceID(event *sdk.Event, candidates ...string) string {
	if event == nil {
		return ""
	}

	if traceID := sdk.GetTraceID(*event); traceID != "" {
		return traceID
	}

	if event.Metadata == nil {
		event.Metadata = make(map[string]string)
	}

	if traceID := FirstNonEmptyTraceID(candidates...); traceID != "" {
		sdk.SetTraceID(event, traceID)
		return traceID
	}

	if traceID := MetadataTraceID(event.Metadata); traceID != "" {
		sdk.SetTraceID(event, traceID)
		return traceID
	}

	traceID := GenerateTraceID()
	sdk.SetTraceID(event, traceID)
	return traceID
}

func parseTraceParent(raw string) string {
	parts := strings.Split(strings.TrimSpace(raw), "-")
	if len(parts) != 4 {
		return ""
	}

	traceID := strings.ToLower(parts[1])
	if len(traceID) != 32 || traceID == strings.Repeat("0", 32) {
		return ""
	}

	if _, err := hex.DecodeString(traceID); err != nil {
		return ""
	}

	return traceID
}
