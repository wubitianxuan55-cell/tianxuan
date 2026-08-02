package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestPNGDataURL(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	out, err := pngDataURL(img)
	if err != nil {
		t.Fatalf("pngDataURL: %v", err)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(out, prefix) {
		t.Fatalf("data URL missing prefix, got %q", out[:min(len(out), 32)])
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(out, prefix))
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	dec, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if dec.Bounds() != img.Bounds() {
		t.Fatalf("bounds mismatch: got %v want %v", dec.Bounds(), img.Bounds())
	}
}
