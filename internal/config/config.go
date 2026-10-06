// Package config хранит настройки в каталоге пользователя.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Config struct {
	SwitchHotkeys    []string `json:"switch_hotkeys"`     // переключение языка, например "Ctrl+Shift"
	ConvertKeys      []string `json:"convert_keys"`       // двойное нажатие — конвертация последнего слова
	DoubleIntervalMs int      `json:"double_interval_ms"` // максимальный интервал двойного нажатия
	Sound            bool     `json:"sound"`              // звук при переключении раскладки
	SwitchEnabled    bool     `json:"switch_enabled"`     // переключение языка сочетаниями
	WordEnabled      bool     `json:"word_enabled"`       // конвертация последнего слова
	SelectionEnabled bool     `json:"selection_enabled"`  // конвертация выделенного текста
	PhraseEnabled    bool     `json:"phrase_enabled"`     // конвертация всей фразы
	PhraseTriple     bool     `json:"phrase_triple"`      // фраза — тройным нажатием клавиши конвертации, иначе двойным нажатием PhraseKeys
	PhraseKeys       []string `json:"phrase_keys"`        // отдельные клавиши конвертации фразы
	ExcludeEnabled   bool     `json:"exclude_enabled"`    // не работать в приложениях из ExcludedApps
	ExcludedApps     []string `json:"excluded_apps"`      // имена исполняемых файлов, например "mstsc.exe"
	UpdateCheck      bool     `json:"update_check"`       // проверять обновления на GitHub
}

// Default возвращает настройки по умолчанию. Load накладывает файл поверх них,
// поэтому поля, которых нет в старом файле, получают эти значения.
func Default() Config {
	return Config{
		SwitchHotkeys:    []string{"Ctrl+Shift"},
		ConvertKeys:      []string{"Shift"},
		DoubleIntervalMs: 400,
		Sound:            true,
		SwitchEnabled:    true,
		WordEnabled:      true,
		SelectionEnabled: true,
		PhraseEnabled:    true,
		PhraseTriple:     true,
		ExcludeEnabled:   true,
		UpdateCheck:      true,
	}
}

// Load читает настройки; если файла нет, возвращает настройки по умолчанию.
func Load() (Config, error) {
	cfg := Default()
	p, err := path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("%s: %w", p, err)
	}
	return cfg, nil
}

func Save(cfg Config) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "langswitch", "config.json"), nil
}
