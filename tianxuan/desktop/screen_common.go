package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
)

// pngDataURL 把图像编码为 PNG data URL（与 SavePastedImage 管线的 dataURL
// 格式一致，可无缝作为图片附件发送）。
func pngDataURL(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
