// Package autostart включает запуск программы при входе пользователя в систему.
// Права администратора не нужны: используются пользовательские настройки ОС.
package autostart

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// executable возвращает полный путь к запущенному исполняемому файлу.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
