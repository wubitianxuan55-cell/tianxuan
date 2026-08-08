//go:build windows

package builtin

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"syscall"
	"unsafe"
)

// GDI/user32 screenshot API (x/sys/windows lacks the GDI functions, so bind
// the DLLs directly). Mirrors the desktop CaptureScreen implementation.
var (
	modUser32 = syscall.NewLazyDLL("user32.dll")
	modGdi32  = syscall.NewLazyDLL("gdi32.dll")

	procGetSystemMetrics   = modUser32.NewProc("GetSystemMetrics")
	procGetDC              = modUser32.NewProc("GetDC")
	procReleaseDC          = modUser32.NewProc("ReleaseDC")
	procCreateCompatibleDC = modGdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = modGdi32.NewProc("DeleteDC")
	procCreateDIBSection   = modGdi32.NewProc("CreateDIBSection")
	procSelectObject       = modGdi32.NewProc("SelectObject")
	procDeleteObject       = modGdi32.NewProc("DeleteObject")
	procBitBlt             = modGdi32.NewProc("BitBlt")
)

const (
	smCxScreen    = 0
	smCyScreen    = 1
	srcCopy       = 0x00CC0020
	dibRGBColors  = 0
	biRGB         = 0
	bitmapInfoLen = 40
)

// bitmapInfo matches the Win32 BITMAPINFOHEADER memory layout (40 bytes).
type bitmapInfo struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

func getSystemMetrics(index int) int {
	r, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int(int32(r))
}

// capturePrimaryScreenPNG captures the primary screen and returns PNG bytes.
func capturePrimaryScreenPNG() ([]byte, error) {
	w := getSystemMetrics(smCxScreen)
	h := getSystemMetrics(smCyScreen)
	if w <= 0 || h <= 0 {
		return nil, errors.New("invalid screen size")
	}

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, errors.New("GetDC failed")
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, errors.New("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)

	bmi := bitmapInfo{
		BiSize:        bitmapInfoLen,
		BiWidth:       int32(w),
		BiHeight:      -int32(h), // top-down DIB: first row is the screen top
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: biRGB,
	}
	var bits unsafe.Pointer
	dib, _, _ := procCreateDIBSection.Call(
		memDC,
		uintptr(unsafe.Pointer(&bmi)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if dib == 0 {
		return nil, errors.New("CreateDIBSection failed")
	}
	defer procDeleteObject.Call(dib)
	if bits == nil {
		return nil, errors.New("DIB section returned nil pixels")
	}

	old, _, _ := procSelectObject.Call(memDC, dib)
	if old == 0 {
		return nil, errors.New("SelectObject failed")
	}
	defer procSelectObject.Call(memDC, old)

	if r, _, _ := procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, 0, 0, srcCopy); r == 0 {
		return nil, errors.New("BitBlt failed")
	}

	// DIB pixels are BGRA; convert to image.RGBA (R/G/B/A).
	pixels := unsafe.Slice((*byte)(bits), w*h*4)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h*4; i += 4 {
		img.Pix[i+0] = pixels[i+2]
		img.Pix[i+1] = pixels[i+1]
		img.Pix[i+2] = pixels[i+0]
		img.Pix[i+3] = pixels[i+3]
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
