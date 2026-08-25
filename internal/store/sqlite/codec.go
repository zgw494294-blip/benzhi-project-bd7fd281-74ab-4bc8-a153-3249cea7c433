package sqlite

import (
	"encoding/json"
	"fmt"
	"time"
)

func encode(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("编码内部值失败: %v", err))
	}
	return string(b)
}

func decode(raw string, target any) error {
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("解码持久化 JSON: %w", err)
	}
	return nil
}

func timeText(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析持久化时间 %q: %w", value, err)
	}
	return t, nil
}
