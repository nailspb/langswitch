//go:build darwin

package platform

/*
#cgo LDFLAGS: -framework ApplicationServices -framework Carbon -framework CoreFoundation -framework AppKit
#include <stdint.h>
int  lsTrusted(void);
int  lsRunTap(void);
void lsSwitchLayout(void);
void lsPostKey(uint16_t code, int down, uint64_t flags);
int  lsLayouts(const uint16_t *codes, int ncodes, uint16_t *out, int maxLayouts, int *current);
void lsSetLayout(int idx);
int  lsActiveApp(char *buf, int n);
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"

	"langswitch/internal/keys"
)

// Типы событий, которые передаёт C-обработчик (см. platform_darwin.c).
const (
	evKeyDown = iota
	evKeyUp
	evFlags
	evMouse
)

// macKeys — виртуальные коды клавиш macOS (kVK_*), привязаны к положению клавиш.
var macKeys = map[uint16]keys.Key{
	0x00: keys.A, 0x01: keys.S, 0x02: keys.D, 0x03: keys.F, 0x04: keys.H, 0x05: keys.G,
	0x06: keys.Z, 0x07: keys.X, 0x08: keys.C, 0x09: keys.V, 0x0A: keys.IntlBackslash, 0x0B: keys.B,
	0x0C: keys.Q, 0x0D: keys.W, 0x0E: keys.E, 0x0F: keys.R, 0x10: keys.Y, 0x11: keys.T,
	0x12: keys.D1, 0x13: keys.D2, 0x14: keys.D3, 0x15: keys.D4, 0x16: keys.D6, 0x17: keys.D5,
	0x18: keys.Equal, 0x19: keys.D9, 0x1A: keys.D7, 0x1B: keys.Minus, 0x1C: keys.D8, 0x1D: keys.D0,
	0x1E: keys.RBracket, 0x1F: keys.O, 0x20: keys.U, 0x21: keys.LBracket, 0x22: keys.I, 0x23: keys.P,
	0x24: keys.Enter, 0x25: keys.L, 0x26: keys.J, 0x27: keys.Quote, 0x28: keys.K, 0x29: keys.Semicolon,
	0x2A: keys.Backslash, 0x2B: keys.Comma, 0x2C: keys.Slash, 0x2D: keys.N, 0x2E: keys.M, 0x2F: keys.Period,
	0x30: keys.Tab, 0x31: keys.Space, 0x32: keys.Grave, 0x33: keys.Backspace, 0x35: keys.Escape,
	0x36: keys.RMeta, 0x37: keys.LMeta, 0x38: keys.LShift, 0x39: keys.CapsLock, 0x3A: keys.LAlt,
	0x3B: keys.LCtrl, 0x3C: keys.RShift, 0x3D: keys.RAlt, 0x3E: keys.RCtrl,
	0x60: keys.F5, 0x61: keys.F6, 0x62: keys.F7, 0x63: keys.F3, 0x64: keys.F8, 0x65: keys.F9,
	0x67: keys.F11, 0x6D: keys.F10, 0x6F: keys.F12, 0x72: keys.Insert, 0x73: keys.Home,
	0x74: keys.PageUp, 0x75: keys.Delete, 0x76: keys.F4, 0x77: keys.End, 0x78: keys.F2,
	0x79: keys.PageDown, 0x7A: keys.F1, 0x7B: keys.Left, 0x7C: keys.Right, 0x7D: keys.Down, 0x7E: keys.Up,
}

// modifierMasks — аппаратно-зависимые флаги NX_DEVICE*KEYMASK для левых и правых модификаторов.
var modifierMasks = map[keys.Key]uint64{
	keys.LCtrl: 0x01, keys.LShift: 0x02, keys.RShift: 0x04, keys.LMeta: 0x08,
	keys.RMeta: 0x10, keys.LAlt: 0x20, keys.RAlt: 0x40, keys.RCtrl: 0x2000,
}

var macCodes = func() map[keys.Key]uint16 {
	m := make(map[keys.Key]uint16, len(macKeys))
	for code, k := range macKeys {
		m[k] = code
	}
	return m
}()

// handler вызывается только из потока event tap.
var handler func(keys.Event) bool

type Backend struct{}

func New() (*Backend, error) {
	if C.lsTrusted() == 0 {
		return nil, errors.New("нужно разрешение: Системные настройки → Конфиденциальность и безопасность → " +
			"Универсальный доступ. Включите LangSwitch и перезапустите программу")
	}
	return &Backend{}, nil
}

func (b *Backend) Run(handle func(keys.Event) bool) error {
	// CFRunLoop привязан к потоку.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	handler = handle
	if C.lsRunTap() == 0 {
		return errors.New("не удалось перехватить клавиатуру: проверьте разрешение «Универсальный доступ»")
	}
	return nil
}

func (b *Backend) SwitchLayout() error {
	C.lsSwitchLayout()
	return nil
}

func (b *Backend) Send(strokes []keys.Stroke) error {
	for _, s := range strokes {
		code, ok := macCodes[s.Key]
		if !ok {
			continue
		}
		var flags C.uint64_t
		if s.Shift {
			flags = flagShift
		}
		C.lsPostKey(C.uint16_t(code), 1, flags)
		C.lsPostKey(C.uint16_t(code), 0, flags)
	}
	return nil
}

//export goHandleEvent
func goHandleEvent(typ, code C.int, flags C.uint64_t, injected C.int) C.int {
	inj := injected != 0
	if typ == evMouse {
		handler(keys.Event{Key: keys.Mouse, Down: true, Injected: inj})
		return 0
	}
	k, ok := macKeys[uint16(code)]
	if typ != evFlags {
		if !ok {
			k = keys.Other
		}
		return cbool(handler(keys.Event{Key: k, Down: typ == evKeyDown, Injected: inj}))
	}
	if k == keys.CapsLock {
		// CapsLock присылает одно событие на каждое переключение — эмулируем нажатие и отпускание.
		swallow := handler(keys.Event{Key: k, Down: true, Injected: inj})
		handler(keys.Event{Key: k, Down: false, Injected: inj})
		return cbool(swallow)
	}
	mask, ok := modifierMasks[k]
	if !ok {
		return 0 // Fn и т.п.
	}
	return cbool(handler(keys.Event{Key: k, Down: uint64(flags)&mask != 0, Injected: inj}))
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

const (
	flagShift   = 0x00020000 // kCGEventFlagMaskShift
	flagCommand = 0x00100000 // kCGEventFlagMaskCommand
	maxLayouts  = 16
)

func (b *Backend) Layouts() ([]keys.Layout, int, error) {
	var codes []C.uint16_t
	var ks []keys.Key
	for code, k := range macKeys {
		if k.IsPrintable() || k == keys.Space {
			codes = append(codes, C.uint16_t(code))
			ks = append(ks, k)
		}
	}
	out := make([]C.uint16_t, maxLayouts*len(codes)*2)
	var cur C.int
	n := int(C.lsLayouts(&codes[0], C.int(len(codes)), &out[0], maxLayouts, &cur))
	if n == 0 {
		return nil, 0, errors.New("не удалось прочитать раскладки")
	}
	return buildLayouts(n, ks, func(i int) rune { return rune(out[i]) }), int(cur), nil
}

func (b *Backend) SetLayout(i int) error {
	C.lsSetLayout(C.int(i))
	return nil
}

func (b *Backend) SendShortcut(k keys.Key) error {
	code, ok := macCodes[k]
	if !ok {
		return fmt.Errorf("нет кода клавиши для %v", k)
	}
	C.lsPostKey(C.uint16_t(code), 1, flagCommand)
	C.lsPostKey(C.uint16_t(code), 0, flagCommand)
	return nil
}

// ActiveApp возвращает имя исполняемого файла активного приложения, например "Terminal".
func (b *Backend) ActiveApp() string {
	var buf [256]C.char
	if C.lsActiveApp(&buf[0], C.int(len(buf))) == 0 {
		return ""
	}
	return C.GoString(&buf[0])
}
