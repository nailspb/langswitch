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
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"langswitch/internal/autostart"
	"langswitch/internal/config"
	"langswitch/internal/engine"
	"langswitch/internal/keys"
	"langswitch/internal/sound"
)

var (
	minWindowSize = fyne.NewSize(780, 560) // окно не сжимается меньше этого размера
	sidebarWidth  = float32(210)
)

type settingsUI struct {
	eng           *engine.Engine
	win           fyne.Window
	cfg           config.Config
	status        *callout
	switchList    *fyne.Container
	convertList   *fyne.Container
	recording     *widget.Button // кнопка, для которой сейчас записывается сочетание
	recordingText string         // исходная надпись этой кнопки

	up         *updater
	phraseList *fyne.Container
	appList    *fyne.Container
	pickButton *widget.Button // «Выбрать приложение»
}

// navPage — пункт бокового меню и его страница.
type navPage struct {
	icon    fyne.Resource
	title   string
	content fyne.CanvasObject
}

// newSettingsUI строит окно настроек. Изменения применяются и сохраняются сразу.
func newSettingsUI(a fyne.App, eng *engine.Engine, cfg config.Config, up *updater) *settingsUI {
	u := &settingsUI{
		eng:         eng,
		win:         a.NewWindow("LangSwitch"),
		cfg:         cfg,
		status:      newCallout(),
		switchList:  container.NewVBox(),
		convertList: container.NewVBox(),
		up:          up,
		phraseList:  container.NewVBox(),
		appList:     container.NewVBox(),
	}
	u.refresh()

	pages := []navPage{
		{theme.ViewRefreshIcon(), "Переключение", u.switchPage()},
		{theme.DocumentCreateIcon(), "Конвертация", u.convertPage()},
		{theme.VisibilityOffIcon(), "Исключения", u.excludePage()},
		{theme.SettingsIcon(), "Общие", u.generalPage()},
	}
	content := container.NewStack(pages[0].content)
	items := make([]*pill, len(pages))
	nav := container.NewVBox()
	for i, p := range pages {
		items[i] = newNavItem(p.icon, p.title, func() {
			for j, it := range items {
				it.setSelected(j == i)
			}
			content.Objects = []fyne.CanvasObject{p.content}
			content.Refresh()
		})
		nav.Add(items[i])
	}
	items[0].selected = true

	body := container.NewBorder(
		container.New(layout.NewCustomPaddedLayout(12, 0, 24, 24), u.status.box), nil, nil, nil, content)
	minSize := canvas.NewRectangle(color.Transparent)
	minSize.SetMinSize(minWindowSize)
	u.win.SetContent(container.NewStack(minSize, container.NewBorder(nil, nil, u.sidebar(nav), nil, body)))
	u.win.Resize(minWindowSize)
	u.win.SetCloseIntercept(func() {
		u.stopRecording()
		u.stopPicking()
		u.win.Hide()
	})
	return u
}

// sidebar — боковая панель: название, меню и общий выключатель внизу.
func (u *settingsUI) sidebar(nav fyne.CanvasObject) fyne.CanvasObject {
	brand := widget.NewLabelWithStyle("LangSwitch", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	brand.SizeName = theme.SizeNameSubHeadingText

	state := hintLabel("Работает")
	setState := func(on bool) {
		if on {
			state.SetText("Работает")
		} else {
			state.SetText("Приостановлено")
		}
	}
	setState(u.cfg.Enabled)
	master := newToggle(u.cfg.Enabled, func(on bool) {
		u.cfg.Enabled = on
		setState(on)
		u.apply()
	})
	footer := container.NewBorder(separator(), nil, nil, nil,
		container.New(layout.NewCustomPaddedLayout(8, 8, 14, 10),
			container.NewBorder(nil, nil, vcenter(master), nil,
				container.New(layout.NewCustomPaddedVBoxLayout(-14), widget.NewLabel("Включено"), state))))

	body := container.NewBorder(
		container.New(layout.NewCustomPaddedLayout(14, 6, 10, 10), brand),
		footer, nil, nil,
		container.New(layout.NewCustomPaddedLayout(0, 0, 10, 10), nav),
	)
	bg := canvas.NewRectangle(colBgElev)
	width := canvas.NewRectangle(color.Transparent)
	width.SetMinSize(fyne.NewSize(sidebarWidth, 0))
	edge := canvas.NewRectangle(colBorder)
	edge.SetMinSize(fyne.NewSize(1, 0))
	return container.NewBorder(nil, nil, nil, edge, container.NewStack(bg, width, body))
}

func (u *settingsUI) switchPage() fyne.CanvasObject {
	add := widget.NewButtonWithIcon("Добавить сочетание", theme.ContentAddIcon(), nil)
	add.OnTapped = func() {
		u.record(add, func(h keys.Hotkey) error {
			u.cfg.SwitchHotkeys = appendUnique(u.cfg.SwitchHotkeys, h.String())
			return nil
		})
	}
	return page("Переключение языка", "Нажатие любого из сочетаний включает следующую раскладку",
		card(u.toggleRow("Переключать по сочетаниям", "", &u.cfg.SwitchEnabled)),
		caption("СОЧЕТАНИЯ КЛАВИШ"),
		card(u.switchList, separator(), container.NewHBox(add)),
	)
}

func (u *settingsUI) convertPage() fyne.CanvasObject {
	add := widget.NewButtonWithIcon("Добавить клавишу", theme.ContentAddIcon(), nil)
	add.OnTapped = func() {
		u.record(add, func(h keys.Hotkey) error {
			if len(h) != 1 {
				return errors.New("для двойного нажатия нужна одна клавиша")
			}
			u.cfg.ConvertKeys = appendUnique(u.cfg.ConvertKeys, h.String())
			return nil
		})
	}
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
	phraseKeys := container.NewVBox(separator(), widget.NewLabel("Клавиши фразы (двойное нажатие)"),
		u.phraseList, container.NewHBox(addPhrase))
	mode := 1
	if u.cfg.PhraseTriple {
		mode = 0
		phraseKeys.Hide()
	}
	modes := segmented([]string{"Тройное нажатие", "Отдельная клавиша"}, mode, func(i int) {
		u.cfg.PhraseTriple = i == 0
		if u.cfg.PhraseTriple {
			phraseKeys.Hide()
		} else {
			phraseKeys.Show()
		}
		u.apply()
	})

	return page("Конвертация текста", "Перевод набранного в другую раскладку: ghbdtn → привет",
		caption("ДВОЙНОЕ НАЖАТИЕ"),
		card(
			u.toggleRow("Последнее слово", "Стирает слово, переключает раскладку и набирает его заново", &u.cfg.WordEnabled),
			separator(),
			u.toggleRow("Выделенный текст",
				"Если после перемещения курсора ничего не набрано — переводит выделение через буфер обмена", &u.cfg.SelectionEnabled),
		),
		caption("КЛАВИШИ КОНВЕРТАЦИИ"),
		card(u.convertList, separator(), container.NewHBox(add)),
		caption("ФРАЗА"),
		card(
			u.toggleRow("Конвертация фразы",
				"Весь текст, набранный после последнего клика, Enter или стрелок", &u.cfg.PhraseEnabled),
			separator(),
			row("Способ", "Третье нажатие клавиши конвертации или двойное нажатие своей клавиши", modes),
			phraseKeys,
		),
	)
}

func (u *settingsUI) excludePage() fyne.CanvasObject {
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
	u.pickButton = widget.NewButtonWithIcon(pickText, theme.SearchIcon(), u.pickApp)

	return page("Исключения", "В этих приложениях LangSwitch ничего не делает",
		card(u.toggleRow("Учитывать исключения",
			"Например, для игр, удалённого рабочего стола или виртуальных машин", &u.cfg.ExcludeEnabled)),
		caption("ПРИЛОЖЕНИЯ"),
		card(
			u.appList,
			separator(),
			container.NewBorder(nil, nil, nil, widget.NewButtonWithIcon("Добавить", theme.ContentAddIcon(), add), entry),
			container.NewHBox(u.pickButton),
		),
	)
}

func (u *settingsUI) generalPage() fyne.CanvasObject {
	intervalLabel := widget.NewLabel(fmt.Sprintf("%d мс", u.cfg.DoubleIntervalMs))
	interval := widget.NewSlider(150, 1000)
	interval.Step = 50
	interval.SetValue(float64(u.cfg.DoubleIntervalMs))
	interval.OnChanged = func(v float64) { intervalLabel.SetText(fmt.Sprintf("%d мс", int(v))) }
	interval.OnChangeEnded = func(v float64) {
		u.cfg.DoubleIntervalMs = int(v)
		u.apply()
	}

	beep := newToggle(u.cfg.Sound, func(on bool) {
		u.cfg.Sound = on
		u.apply()
		if on {
			sound.Play()
		}
	})

	auto := newToggle(autostart.Enabled(), nil)
	auto.OnChanged = func(on bool) {
		if err := autostart.Set(on); err != nil {
			u.status.set("Автозапуск: " + err.Error())
			auto.On = !on
			auto.Refresh()
			return
		}
		u.status.set("")
	}

	return page("Общие", "",
		card(
			row("Интервал между нажатиями", "Сколько ждать следующего нажатия клавиши конвертации",
				fixedWidth(240, container.NewBorder(nil, nil, nil, intervalLabel, interval))),
			separator(),
			row("Звук при переключении", "Короткий сигнал при смене раскладки", beep),
			separator(),
			row("Запускать при входе в систему", "", auto),
		),
		caption("ОБНОВЛЕНИЯ"),
		card(
			u.toggleRow("Проверять обновления", "Раз в сутки, через GitHub Releases", &u.cfg.UpdateCheck),
			separator(),
			u.updateRow(),
		),
	)
}

// refresh перестраивает списки из u.cfg.
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

// rows строит строки списка: плашка с названием и кнопка удаления.
func (u *settingsUI) rows(items []string, remove func(i int)) []fyne.CanvasObject {
	if len(items) == 0 {
		empty := widget.NewLabel("Не задано")
		empty.Importance = widget.LowImportance
		return []fyne.CanvasObject{empty}
	}
	rows := make([]fyne.CanvasObject, 0, 2*len(items))
	for i, item := range items {
		del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			remove(i)
			u.apply()
		})
		del.Importance = widget.LowImportance
		if i > 0 {
			rows = append(rows, separator())
		}
		rows = append(rows, container.NewBorder(nil, nil, nil, del, container.NewHBox(vcenter(chip(item)))))
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
		u.status.set("Ошибка: " + err.Error())
		return
	}
	u.eng.SetSettings(s)
	u.up.enabled.Store(u.cfg.UpdateCheck)
	u.status.set("")
}

// record записывает сочетание с клавиатуры и передаёт его в add. Esc отменяет запись.
func (u *settingsUI) record(btn *widget.Button, add func(keys.Hotkey) error) {
	u.stopRecording()
	u.recording, u.recordingText = btn, btn.Text
	btn.SetText("Нажмите клавиши… (Esc — отмена)")
	btn.Importance = widget.HighImportance
	btn.Refresh()
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
				u.status.set(err.Error())
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
		u.recording.Importance = widget.MediumImportance
		u.recording.Refresh()
		u.recording = nil
	}
}

func appendUnique(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

// toggleRow — строка настройки с переключателем, привязанным к полю настроек.
func (u *settingsUI) toggleRow(title, hint string, field *bool) fyne.CanvasObject {
	return row(title, hint, newToggle(*field, func(on bool) {
		*field = on
		u.apply()
	}))
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
	u.pickButton.Importance = widget.HighImportance
	u.pickButton.Refresh()
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
		u.pickButton.Importance = widget.MediumImportance
		u.pickButton.Refresh()
	}
}

// updateRow — строка с версией программы и кнопкой проверки обновлений.
func (u *settingsUI) updateRow() fyne.CanvasObject {
	status := hintLabel("Нажмите «Проверить», чтобы узнать о новой версии")
	link := widget.NewHyperlink("", nil)
	link.Hide()
	var check *widget.Button
	check = widget.NewButton("Проверить", func() {
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
					status.SetText("Установлена последняя версия")
				}
			})
		}()
	})
	return rowWith("Версия "+version, status, container.NewHBox(link, check))
}
