//go:build !windows

package builtin

import "errors"

// capturePrimaryScreenPNG is unsupported off Windows (the desktop build target).
func capturePrimaryScreenPNG() ([]byte, error) {
	return nil, errors.New("capture_screen is only supported on Windows")
}
