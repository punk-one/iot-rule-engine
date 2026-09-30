package dispatcher

import (
	"encoding/json"
	"fmt"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

// DecodeEvent 从字节数组解码 Event
func DecodeEvent(data []byte) (sdk.Event, error) {
	var e sdk.Event
	err := json.Unmarshal(data, &e)
	return e, err
}

// EncodeEvent 将 Event 编码为字节数组
func EncodeEvent(e sdk.Event) ([]byte, error) {
	return json.Marshal(e)
}

// CheckEventPayloadSize 检查 Event Payload 大小
// maxPayloadKB: 最大 Payload 大小（KB），0 表示不限制
func CheckEventPayloadSize(event sdk.Event, maxPayloadKB int) error {
	if maxPayloadKB <= 0 {
		return nil // 未配置，不检查
	}

	// 编码事件以获取实际大小
	data, err := EncodeEvent(event)
	if err != nil {
		return fmt.Errorf("failed to encode event for size check: %w", err)
	}

	sizeKB := len(data) / 1024
	if sizeKB > maxPayloadKB {
		return fmt.Errorf("%w: event size %dKB exceeds limit %dKB", ErrPayloadTooLarge, sizeKB, maxPayloadKB)
	}

	return nil
}
