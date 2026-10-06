package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed assets/icon.png
var iconPNG []byte

// appIcon — иконка приложения для трея и окон. Генерируется: make icon.
var appIcon = fyne.NewStaticResource("langswitch.png", iconPNG)

//go:embed assets/tray.png
var trayPNG []byte

// trayIcon — упрощённая иконка для трея: крупная A, различимая в 16 px. Генерируется: make icon.
var trayIcon = fyne.NewStaticResource("langswitch-tray.png", trayPNG)
