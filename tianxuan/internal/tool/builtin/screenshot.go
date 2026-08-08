package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(captureScreen{}) }

// captureScreenShot captures the primary screen as PNG bytes. A package-level
// variable so tests can inject a deterministic fake without touching real GDI.
var captureScreenShot = capturePrimaryScreenPNG

// captureScreen lets the model take its own screenshot: capture the primary
// screen, save it under .tianxuan/attachments, and return the @-reference path
// so the vision skill can read it back — the agent-side analogue of the desktop
// Composer's camera button (CaptureScreen), wired as a model-callable tool.
type captureScreen struct{}

func (captureScreen) Name() string { return "capture_screen" }

func (captureScreen) Description() string {
	return "Capture the primary screen as a PNG saved under .tianxuan/attachments/ and return its relative @-reference path. Use when the user asks you to screenshot the screen/UI or wants you to see what is currently displayed (then read the image with the vision skill via run_skill name=vision)."
}

func (captureScreen) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (captureScreen) CompactDescription() string { return "截取主屏幕存为附件，返回 @路径（配合 vision 技能识图）" }
func (captureScreen) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (captureScreen) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	// Validate the args are legal JSON (schema has no fields; the agent layer
	// runs the full ValidateArgs pass before execution).
	var anyArgs map[string]any
	if err := json.Unmarshal(args, &anyArgs); err != nil {
		return "", fmt.Errorf("capture_screen: invalid args: %w", err)
	}
	png, err := captureScreenShot()
	if err != nil {
		return "", fmt.Errorf("capture_screen: %w", err)
	}
	if len(png) == 0 {
		return "", errors.New("capture_screen: captured image is empty")
	}
	rel, err := saveScreenAttachment(png)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"Screen captured: @%s (%d bytes). Image bytes are not inlined — invoke the vision skill (run_skill name=vision) with this path to see the screen content.",
		rel, len(png),
	), nil
}

// ReadOnly is false: the tool writes a PNG file under .tianxuan/attachments.
func (captureScreen) ReadOnly() bool { return false }

// saveScreenAttachment writes the PNG under .tianxuan/attachments with a
// timestamped name and returns the relative @-reference path (same convention
// as the paste/capture attachment pipeline, so @ref resolution and the vision
// skill both work unchanged).
func saveScreenAttachment(png []byte) (string, error) {
	dir := filepath.Join(".tianxuan", "attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("capture_screen: create attachment dir: %w", err)
	}
	name := fmt.Sprintf("screen-%s.png", time.Now().Format("20060102-150405.000000"))
	rel := filepath.Join(dir, name)
	if err := os.WriteFile(rel, png, 0o644); err != nil {
		return "", fmt.Errorf("capture_screen: write attachment: %w", err)
	}
	return filepath.ToSlash(rel), nil
}
