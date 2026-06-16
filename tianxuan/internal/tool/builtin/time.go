package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(timeTool{}) }

type timeTool struct{}

func (timeTool) Name() string { return "time" }

func (timeTool) Description() string {
	return "Get the current date/time in the agent's configured timezone. Use this when you need to know today's date, the current time, or the day of the week — don't guess or ask the user."
}

func (timeTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "timezone":{"type":"string","description":"IANA timezone (e.g. 'Asia/Shanghai', 'America/New_York'). Empty defaults to UTC.","default":""}
},
"required":[]
}`)
}

type timeArgs struct {
	Timezone string `json:"timezone"`
}

func (timeTool) ReadOnly() bool { return true }

func (timeTool) CompactDescription() string { return "获取当前日期时间(可指定时区)" }
func (timeTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"timezone":{"type":"string"}}}`)
}

func (timeTool) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var args timeArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("time: invalid args: %w", err)
	}
	loc := time.UTC
	if args.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(args.Timezone)
		if err != nil {
			return "", fmt.Errorf("time: unknown timezone %q: %w", args.Timezone, err)
		}
	}
	now := time.Now().In(loc)
	return fmt.Sprintf("Current time: %s\nUnix timestamp: %d\nTimezone: %s\nWeekday: %s",
		now.Format("2006-01-02 15:04:05 MST"),
		now.Unix(),
		now.Location().String(),
		now.Weekday().String()), nil
}
