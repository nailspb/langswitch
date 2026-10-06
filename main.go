// LangSwitch — переключение языка по горячей клавише и конвертация последнего слова
// по двойному нажатию клавиши. Работает в трее, права администратора не нужны.
package main

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"

	"langswitch/internal/config"
	"langswitch/internal/engine"
	"langswitch/internal/keys"
	"langswitch/internal/platform"
	"langswitch/internal/update"
)

func main() {
	a := app.NewWithID("io.github.langswitch")
	desk, ok := a.(desktop.App)
	if !ok {
		log.Fatal("системный трей не поддерживается")
	}

	a.Settings().SetTheme(appTheme{})

	cfg, err := config.Load()
	if err != nil {
		log.Printf("настройки: %v; используются настройки по умолчанию", err)
	}
	s, err := toSettings(cfg)
	if err != nil {
		log.Printf("настройки: %v; используются настройки по умолчанию", err)
		cfg = config.Default()
		s, _ = toSettings(cfg)
	}

	backend, err := platform.New()
	if err != nil {
		showFatal(a, err)
		a.Run()
		return
	}
	eng := engine.New(backend, newClipboard(a), s)
	up := &updater{}
	up.enabled.Store(cfg.UpdateCheck)
	ui := newSettingsUI(a, eng, cfg, up)

	quit := fyne.NewMenuItem("Выход", a.Quit)
	quit.IsQuit = true
	settingsItem := fyne.NewMenuItem("Настройки…", ui.win.Show)
	setMenu := func(extra ...*fyne.MenuItem) {
		desk.SetSystemTrayMenu(fyne.NewMenu("LangSwitch", slices.Concat(extra, []*fyne.MenuItem{settingsItem, quit})...))
	}
	a.SetIcon(appIcon)
	desk.SetSystemTrayIcon(trayIcon)
	setMenu()
	desk.SetSystemTrayWindow(ui.win) // левый клик по значку открывает настройки, правый — меню

	var notified string // версия, о которой уже сообщили
	up.found = func(rel update.Release) {
		if rel.Version == notified {
			return
		}
		notified = rel.Version
		setMenu(fyne.NewMenuItem("Скачать версию "+rel.Version+"…", func() { openURL(a, rel.URL) }),
			fyne.NewMenuItemSeparator())
		a.SendNotification(fyne.NewNotification("LangSwitch", "Доступна новая версия "+rel.Version))
	}
	go up.run()
	// Fyne не задаёт подсказку значка; трей запускается до OnStarted, поэтому ставим её здесь.
	a.Lifecycle().SetOnStarted(func() { systray.SetTooltip("LangSwitch") })

	go func() {
		if err := eng.Run(); err != nil {
			fyne.Do(func() { showFatal(a, err) })
		}
	}()
	a.Run()
}

func toSettings(c config.Config) (engine.Settings, error) {
	if c.DoubleIntervalMs < 100 || c.DoubleIntervalMs > 2000 {
		return engine.Settings{}, errors.New("интервал двойного нажатия должен быть от 100 до 2000 мс")
	}
	s := engine.Settings{
		Interval:     time.Duration(c.DoubleIntervalMs) * time.Millisecond,
		Sound:        c.Sound,
		WordOn:       c.WordEnabled,
		SelectionOn:  c.SelectionEnabled,
		PhraseOn:     c.PhraseEnabled,
		PhraseTriple: c.PhraseTriple,
	}
	for _, v := range c.SwitchHotkeys {
		h, err := keys.ParseHotkey(v)
		if err != nil {
			return engine.Settings{}, fmt.Errorf("переключение языка: %w", err)
		}
		if c.SwitchEnabled {
			s.Switch = append(s.Switch, h)
		}
	}
	var err error
	if s.Convert, err = parseKeys(c.ConvertKeys); err != nil {
		return engine.Settings{}, fmt.Errorf("конвертация: %w", err)
	}
	if s.Phrase, err = parseKeys(c.PhraseKeys); err != nil {
		return engine.Settings{}, fmt.Errorf("конвертация фразы: %w", err)
	}
	if c.ExcludeEnabled {
		s.Excluded = c.ExcludedApps
	}
	if !c.Enabled {
		s.Switch, s.WordOn, s.SelectionOn, s.PhraseOn, s.Excluded = nil, false, false, false, nil
	}
	return s, nil
}

func parseKeys(list []string) ([]keys.Key, error) {
	var out []keys.Key
	for _, v := range list {
		k, ok := keys.Parse(strings.TrimSpace(v))
		if !ok {
			return nil, fmt.Errorf("нужна одна клавиша, получено %q", v)
		}
		out = append(out, k)
	}
	return out, nil
}

func showFatal(a fyne.App, err error) {
	w := a.NewWindow("LangSwitch")
	msg := widget.NewLabel(err.Error())
	msg.Wrapping = fyne.TextWrapWord
	w.SetContent(container.NewVBox(msg, widget.NewButton("Выход", a.Quit)))
	w.Resize(fyne.NewSize(420, 0))
	w.SetOnClosed(a.Quit)
	w.Show()
}

func openURL(a fyne.App, s string) {
	u, err := url.Parse(s)
	if err != nil {
		log.Printf("ссылка %q: %v", s, err)
		return
	}
	if err := a.OpenURL(u); err != nil {
		log.Printf("ссылка %q: %v", s, err)
	}
}
