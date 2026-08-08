package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// withScreenCaptureFn swaps the injectable capture function for the duration
// of a test and runs it inside a fresh temp cwd so attachment writes are
// isolated. It returns the temp cwd so the caller can read saved files by
// absolute path after the cwd is restored.
func withScreenCaptureFn(t *testing.T, fn func() ([]byte, error), run func() (string, error)) (string, string, error) {
	t.Helper()
	orig := captureScreenShot
	captureScreenShot = fn
	defer func() { captureScreenShot = orig }()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	got, err := run()
	return tmp, got, err
}

func TestCaptureScreenSavesAttachment(t *testing.T) {
	want := testPNG()
	tmp, got, err := withScreenCaptureFn(t, func() ([]byte, error) { return want, nil }, func() (string, error) {
		return (captureScreen{}).Execute(context.Background(), json.RawMessage(`{}`))
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Result must expose the @-reference path and point the model at the vision skill.
	if !strings.Contains(got, "@.tianxuan/attachments/screen-") {
		t.Fatalf("result %q missing attachment @-path", got)
	}
	if !strings.Contains(got, "run_skill") || !strings.Contains(got, "vision") {
		t.Fatalf("result %q missing vision-skill pointer", got)
	}
	// The saved file must exist under .tianxuan/attachments with identical bytes.
	rel := strings.TrimPrefix(got, "Screen captured: @")
	rel = strings.SplitN(rel, " ", 2)[0]
	raw, err := os.ReadFile(filepath.Join(tmp, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("saved attachment unreadable: %v", err)
	}
	if !bytes.Equal(raw, want) {
		t.Fatal("saved attachment bytes differ from captured PNG")
	}
	if filepath.ToSlash(filepath.Dir(rel)) != ".tianxuan/attachments" {
		t.Fatalf("attachment saved outside .tianxuan/attachments: %s", rel)
	}
}

func TestCaptureScreenPropagatesCaptureError(t *testing.T) {
	_, _, err := withScreenCaptureFn(t, func() ([]byte, error) { return nil, errors.New("GetDC failed") }, func() (string, error) {
		return (captureScreen{}).Execute(context.Background(), json.RawMessage(`{}`))
	})
	if err == nil || !strings.Contains(err.Error(), "GetDC failed") {
		t.Fatalf("expected capture error to propagate, got %v", err)
	}
}

func TestCaptureScreenRejectsBadArgs(t *testing.T) {
	_, _, err := withScreenCaptureFn(t, func() ([]byte, error) { return testPNG(), nil }, func() (string, error) {
		return (captureScreen{}).Execute(context.Background(), json.RawMessage(`not json`))
	})
	if err == nil {
		t.Fatal("expected invalid JSON args to fail loudly")
	}
}
