//go:build darwin || linux

package autostart

import (
	"os"
	"path/filepath"
)

// Enabled сообщает, включён ли автозапуск.
func Enabled() bool {
	p, err := file()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Set включает или выключает автозапуск текущего исполняемого файла.
func Set(on bool) error {
	p, err := file()
	if err != nil {
		return err
	}
	if !on {
		return removeIfExists(p)
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content(exe)), 0o644)
}
