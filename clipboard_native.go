//go:build windows || darwin

package main

import (
	"fyne.io/fyne/v2"

	"langswitch/internal/clipboard"
	"langswitch/internal/engine"
)

// newClipboard возвращает буфер обмена с сохранением всех форматов.
func newClipboard(fyne.App) engine.Clipboard { return clipboard.New() }
