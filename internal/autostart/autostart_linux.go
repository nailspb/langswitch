//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// file — ярлык XDG Autostart пользователя.
func file() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "langswitch.desktop"), nil
}

func content(exe string) string {
	quoted := `"` + strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", "$", `\\$`).Replace(exe) + `"`
	return fmt.Sprintf("[Desktop Entry]\nType=Application\nName=LangSwitch\nExec=%s\nX-GNOME-Autostart-enabled=true\n", quoted)
}
