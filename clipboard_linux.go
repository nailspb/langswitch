//go:build linux

package main

import (
	"fyne.io/fyne/v2"

	"langswitch/internal/engine"
)

// В X11 содержимое буфера обмена хранит программа-владелец, поэтому сохранить
// его целиком нельзя — восстанавливается только текст.
func newClipboard(a fyne.App) engine.Clipboard { return fyneClipboard{a} }

// fyneClipboard — буфер обмена через Fyne. Методы вызываются из рабочей горутины
// движка, а Fyne работает с буфером только в главном потоке.
type fyneClipboard struct{ app fyne.App }

func (c fyneClipboard) Text() (s string) {
	fyne.DoAndWait(func() { s = c.app.Clipboard().Content() })
	return s
}

func (c fyneClipboard) SetText(s string) {
	fyne.DoAndWait(func() { c.app.Clipboard().SetContent(s) })
}

func (c fyneClipboard) Save() (restore func()) {
	old := c.Text()
	return func() { c.SetText(old) }
}
