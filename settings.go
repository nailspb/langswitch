package main

import (
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"langswitch/internal/autostart"
	"langswitch/internal/config"
	"langswitch/internal/engine"
	"langswitch/internal/keys"
	"langswitch/internal/sound"
)

// Окно не сжимается меньше этого размера.
var minWindowSize = fyne.NewSize(520, 520)

type settingsUI struct {
	eng           *engine.Engine
	win           fyne.Window
	cfg           config.Config
	status        *widget.Label
	switchList    *fyne.Container
	convertList   *fyne.Container
	recording     *widget.Button // кнопка, для которой сейчас записывается сочетание
	recordingText string         // исходная надпись этой кнопки

	up         *updater
	phraseList *fyne.Container
	appList    *fyne.Container
	pickButton *widget.Button // «Выбрать приложение»
}

// newSettingsUI строит окно настроек. Изменения применяются и сохраняются сразу.
func newSettingsUI(a fyne.App, eng *engine.Engine, cfg config.Config, up *updater) *settingsUI {
	u := &settingsUI{
		eng:         eng,
		win:         a.NewWindow("LangSwitch — настройки"),
		cfg:         cfg,
		status:      widget.NewLabel(""),
		switchList:  container.NewVBox(),
		convertList: container.NewVBox(),
		up:          up,
		phraseList:  container.NewVBox(),
		appList:     container.NewVBox(),
	}
	u.status.Importance = widget.DangerImportance
	u.status.Wrapping = fyne.TextWrapWord
	u.refresh()

	addSwitch := widget.NewButtonWithIcon("Добавить сочетание", theme.ContentAddIcon(), nil)
	addSwitch.OnTapped = func() {
		u.record(addSwitch, func(h keys.Hotkey) error {
			u.cfg.SwitchHotkeys = appendUnique(u.cfg.SwitchHotkeys, h.String())
			return nil
		})
	}
	addConvert := widget.NewButtonWithIcon("Добавить клавишу", theme.ContentAddIcon(), nil)
	addConvert.OnTapped = func() {
		u.record(addConvert, func(h keys.Hotkey) error {
			if len(h) != 1 {
				return errors.New("для двойного нажатия нужна одна клавиша")
			}
			u.cfg.ConvertKeys = appendUnique(u.cfg.ConvertKeys, h.String())
			return nil
		})
	}

	intervalLabel := widget.NewLabel(fmt.Sprintf("%d мс", cfg.DoubleIntervalMs))
	interval := widget.NewSlider(150, 1000)
	interval.Step = 50
	interval.SetValue(float64(cfg.DoubleIntervalMs))
	interval.OnChanged = func(v float64) { intervalLabel.SetText(fmt.Sprintf("%d мс", int(v))) }
	interval.OnChangeEnded = func(v float64) {
		u.cfg.DoubleIntervalMs = int(v)
		u.apply()
	}

	soundCheck := widget.NewCheck("Звук при переключении раскладки", nil)
	soundCheck.SetChecked(cfg.Sound)
	soundCheck.OnChanged = func(on bool) {
		u.cfg.Sound = on
		u.apply()
		if on {
			sound.Play()
		}
	}

	autostartCheck := widget.NewCheck("Запускать при входе в систему", nil)
	autostartCheck.SetChecked(autostart.Enabled())
	autostartCheck.OnChanged = func(on bool) {
		if err := autostart.Set(on); err != nil {
			u.status.SetText("Автозапуск: " + err.Error())
			return
		}
		u.status.SetText("")
	}

	content := container.NewVBox(
		widget.NewCard("Переключение языка", "Нажатие любого из сочетаний включает следующую раскладку",
			container.NewVBox(u.toggle("Включено", &u.cfg.SwitchEnabled), u.switchList, addSwitch)),
		widget.NewCard("Конвертация текста",
			"Двойное нажатие клавиши переводит в другую раскладку последнее набранное слово, а если его нет — выделенный текст",
			container.NewVBox(
				u.toggle("Последнее слово", &u.cfg.WordEnabled),
				u.toggle("Выделенный текст", &u.cfg.SelectionEnabled),
				u.convertList, addConvert,
			)),
		u.phraseCard(),
		u.excludeCard(),
		widget.NewCard("Дополнительно", "", container.NewVBox(
			container.NewBorder(nil, nil, widget.NewLabel("Интервал между нажатиями"), intervalLabel, interval),
			soundCheck,
			autostartCheck,
			u.updateBox(),
		)),
		u.status,
	)

	minSize := canvas.NewRectangle(color.Transparent)
	minSize.SetMinSize(minWindowSize)
	u.win.SetContent(container.NewStack(minSize, container.NewVScroll(container.NewPadded(content))))
	u.win.Resize(minWindowSize)
	u.win.SetCloseIntercept(func() {
		u.stopRecording()
		u.stopPicking()
		u.win.Hide()
	})
	return u
}

// refresh перестраивает списки сочетаний из u.cfg.
func (u *settingsUI) refresh() {
	u.switchList.Objects = u.rows(u.cfg.SwitchHotkeys, func(i int) {
		u.cfg.SwitchHotkeys = slices.Delete(u.cfg.SwitchHotkeys, i, i+1)
	})
	u.switchList.Refresh()
	u.convertList.Objects = u.rows(u.cfg.ConvertKeys, func(i int) {
		u.cfg.ConvertKeys = slices.Delete(u.cfg.ConvertKeys, i, i+1)
	})
	u.convertList.Refresh()
	u.phraseList.Objects = u.rows(u.cfg.PhraseKeys, func(i int) {
		u.cfg.PhraseKeys = slices.Delete(u.cfg.PhraseKeys, i, i+1)
	})
	u.phraseList.Refresh()
	u.appList.Objects = u.rows(u.cfg.ExcludedApps, func(i int) {
		u.cfg.ExcludedApps = slices.Delete(u.cfg.ExcludedApps, i, i+1)
	})
	u.appList.Refresh()
}

func (u *settingsUI) rows(items []string, remove func(i int)) []fyne.CanvasObject {
	if len(items) == 0 {
		empty := widget.NewLabel("Не задано")
		empty.Importance = widget.LowImportance
		return []fyne.CanvasObject{empty}
	}
	rows := make([]fyne.CanvasObject, 0, len(items))
	for i, item := range items {
		name := widget.NewLabel(item)
		name.TextStyle.Bold = true
		del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			remove(i)
			u.apply()
		})
		del.Importance = widget.LowImportance
		rows = append(rows, container.NewBorder(nil, nil, nil, del, name))
	}
	return rows
}

// apply проверяет настройки, передаёт их движку и сохраняет на диск.
func (u *settingsUI) apply() {
	u.refresh()
	s, err := toSettings(u.cfg)
	if err == nil {
		err = config.Save(u.cfg)
	}
	if err != nil {
		u.status.SetText("Ошибка: " + err.Error())
		return
	}
	u.eng.SetSettings(s)
	u.up.enabled.Store(u.cfg.UpdateCheck)
	u.status.SetText("")
}

// record записывает сочетание с клавиатуры и передаёт его в add. Esc отменяет запись.
func (u *settingsUI) record(btn *widget.Button, add func(keys.Hotkey) error) {
	u.stopRecording()
	u.recording, u.recordingText = btn, btn.Text
	btn.SetText("Нажмите клавиши… (Esc — отмена)")
	u.eng.Capture(func(h keys.Hotkey) {
		fyne.Do(func() {
			if u.recording != btn {
				return
			}
			u.stopRecording()
			if len(h) == 1 && h[0] == keys.Escape {
				return
			}
			if err := add(h); err != nil {
				u.status.SetText(err.Error())
				return
			}
			u.apply()
		})
	})
}

func (u *settingsUI) stopRecording() {
	u.eng.CancelCapture()
	if u.recording != nil {
		u.recording.SetText(u.recordingText)
		u.recording = nil
	}
}

func appendUnique(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

// toggle создаёт флажок, привязанный к полю настроек.
func (u *settingsUI) toggle(label string, field *bool) *widget.Check {
	c := widget.NewCheck(label, func(on bool) {
		*field = on
		u.apply()
	})
	c.Checked = *field // без SetChecked, чтобы не вызвать сохранение при построении окна
	return c
}

func (u *settingsUI) phraseCard() fyne.CanvasObject {
	const (
		tripleMode = "Тройное нажатие клавиши конвертации"
		keyMode    = "Двойное нажатие отдельной клавиши"
	)
	addPhrase := widget.NewButtonWithIcon("Добавить клавишу", theme.ContentAddIcon(), nil)
	addPhrase.OnTapped = func() {
		u.record(addPhrase, func(h keys.Hotkey) error {
			if len(h) != 1 {
				return errors.New("для двойного нажатия нужна одна клавиша")
			}
			u.cfg.PhraseKeys = appendUnique(u.cfg.PhraseKeys, h.String())
			return nil
		})
	}
	phraseKeys := container.NewVBox(u.phraseList, addPhrase)

	mode := widget.NewRadioGroup([]string{tripleMode, keyMode}, nil)
	mode.Required = true
	mode.Selected = keyMode
	if u.cfg.PhraseTriple {
		mode.Selected = tripleMode
		phraseKeys.Hide()
	}
	mode.OnChanged = func(v string) {
		u.cfg.PhraseTriple = v == tripleMode
		if u.cfg.PhraseTriple {
			phraseKeys.Hide()
		} else {
			phraseKeys.Show()
		}
		u.apply()
	}
	return widget.NewCard("Конвертация фразы",
		"Переводит в другую раскладку весь текст, набранный после последнего перемещения курсора (клика, Enter, стрелок)",
		container.NewVBox(u.toggle("Включено", &u.cfg.PhraseEnabled), mode, phraseKeys))
}

func (u *settingsUI) excludeCard() fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Имя программы, например mstsc.exe")
	add := func() {
		if name := strings.TrimSpace(entry.Text); name != "" {
			u.cfg.ExcludedApps = appendUnique(u.cfg.ExcludedApps, name)
			entry.SetText("")
			u.apply()
		}
	}
	entry.OnSubmitted = func(string) { add() }
	addButton := widget.NewButtonWithIcon("", theme.ContentAddIcon(), add)

	u.pickButton = widget.NewButtonWithIcon(pickText, theme.SearchIcon(), u.pickApp)
	return widget.NewCard("Исключения", "В этих приложениях LangSwitch ничего не делает",
		container.NewVBox(
			u.toggle("Включено", &u.cfg.ExcludeEnabled),
			u.appList,
			container.NewBorder(nil, nil, nil, addButton, entry),
			u.pickButton,
		))
}

const pickText = "Выбрать приложение"

// pickApp ждёт нажатия клавиши в другом приложении и добавляет его в исключения.
// Повторное нажатие кнопки отменяет выбор.
func (u *settingsUI) pickApp() {
	if u.pickButton.Text != pickText {
		u.stopPicking()
		return
	}
	u.pickButton.SetText("Нажмите клавишу в нужном приложении…")
	u.eng.CaptureApp(func(app string) {
		fyne.Do(func() {
			u.stopPicking()
			u.cfg.ExcludedApps = appendUnique(u.cfg.ExcludedApps, app)
			u.apply()
		})
	})
}

func (u *settingsUI) stopPicking() {
	u.eng.CancelCaptureApp()
	if u.pickButton != nil {
		u.pickButton.SetText(pickText)
	}
}

func (u *settingsUI) updateBox() fyne.CanvasObject {
	status := widget.NewLabel("Версия " + version)
	link := widget.NewHyperlink("", nil)
	link.Hide()
	var check *widget.Button
	check = widget.NewButton("Проверить сейчас", func() {
		check.Disable()
		status.SetText("Проверка…")
		go func() {
			rel, newer, err := u.up.check()
			fyne.Do(func() {
				check.Enable()
				switch {
				case err != nil:
					status.SetText("Не удалось проверить: " + err.Error())
				case newer:
					status.SetText("Доступна версия " + rel.Version)
					if err := link.SetURLFromString(rel.URL); err == nil {
						link.SetText("Скачать")
						link.Show()
					}
					u.up.found(rel)
				default:
					status.SetText("Установлена последняя версия (" + version + ")")
				}
			})
		}()
	})
	return container.NewVBox(
		u.toggle("Проверять обновления", &u.cfg.UpdateCheck),
		container.NewBorder(nil, nil, nil, check, container.NewHBox(status, link)),
	)
}
