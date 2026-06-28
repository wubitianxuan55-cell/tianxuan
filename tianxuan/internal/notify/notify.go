// Package notify provides cross-platform desktop notifications with zero
// external Go dependencies. It shells out to the platform's native notification
// mechanism:
//   - macOS: osascript (native Notification Center)
//   - Linux: notify-send (libnotify)
//   - Windows: powershell (Windows.UI.Notifications)
//
// The package degrades gracefully: when no mechanism is available it silently
// does nothing, so notifications are a nice-to-have, never a hard requirement.
package notify

import (
	"fmt"
	"os/exec"
	"runtime"

	"tianxuan/internal/proc"
)

// Send dispatches a desktop notification. It returns nil when the notification
// was sent (or skipped because no mechanism is available), and an error only
// when the notification mechanism itself failed in an unexpected way.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		return sendMacOS(title, body)
	case "linux":
		return sendLinux(title, body)
	case "windows":
		return sendWindows(title, body)
	}
	return nil
}

func sendMacOS(title, body string) error {
	script := fmt.Sprintf(`display notification %q with title %q`,
		escape(body), escape(title))
	return exec.Command("osascript", "-e", script).Run()
}

func sendLinux(title, body string) error {
	cmd := exec.Command("notify-send", title, body)
	// notify-send may fail if $DISPLAY / WAYLAND_DISPLAY is unset (e.g. SSH).
	// We ignore that — the notification is best-effort.
	_ = cmd.Run()
	return nil
}

func sendWindows(title, body string) error {
	// Try PowerShell-based toast notification.
	script := fmt.Sprintf(
		`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null;
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02);
$textNodes = $template.GetElementsByTagName("text");
$textNodes.Item(0).AppendChild($template.CreateTextNode(%q)) > $null;
$textNodes.Item(1).AppendChild($template.CreateTextNode(%q)) > $null;
$toast = [Windows.UI.Notifications.ToastNotification]::new($template);
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier().Show($toast);`,
		escape(title), escape(body),
	)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	proc.HideWindow(cmd) // Windows: 防止弹出 cmd 黑框
	// Ignore errors: may fail on older Windows / non-UWP environments.
	_ = cmd.Run()
	return nil
}

func escape(s string) string {
	// Escape backslashes, double quotes, and newlines for the shell / script.
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			result = append(result, '\\', '\\')
		case '"':
			result = append(result, '\\', '"')
		case '\n':
			result = append(result, '\\', 'n')
		case '\r':
			result = append(result, '\\', 'r')
		default:
			result = append(result, s[i])
		}
	}
	return string(result)
}
