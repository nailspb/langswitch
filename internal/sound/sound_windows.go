//go:build windows

package sound

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procPlaySoundW = windows.NewLazySystemDLL("winmm.dll").NewProc("PlaySoundW")

const (
	sndAsync     = 0x0001
	sndNoDefault = 0x0002
	sndMemory    = 0x0004
)

// Play проигрывает сигнал, не задерживая вызывающего: открытие аудиоустройства
// (особенно Bluetooth) может занимать заметное время. click живёт всё время работы
// программы, поэтому его память остаётся валидной до конца воспроизведения.
func Play() {
	go procPlaySoundW.Call(uintptr(unsafe.Pointer(&click[0])), 0, sndAsync|sndNoDefault|sndMemory)
}
