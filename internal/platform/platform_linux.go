//go:build linux

package platform

/*
#cgo LDFLAGS: -lX11 -lXtst -lxkbcommon
#include <stdint.h>
int  lsOpen(void);
int  lsRunRecord(void);
int  lsSwitchLayout(void);
void lsFakeKey(int code, int down);
void lsFlush(void);
int  lsLayouts(const int *codes, int ncodes, uint32_t *out, int maxLayouts, int *current);
void lsSetLayout(int group);
int  lsActivePID(void);
*/
import "C"

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"langswitch/internal/keys"
)

// Коды клавиш X11 = коды evdev + 8.
const evdevOffset = 8

// linuxKeys — коды evdev: печатные совпадают со скан-кодами Set 1, остальные добавлены.
var linuxKeys = func() map[uint32]keys.Key {
	m := maps.Clone(scanKeys)
	maps.Copy(m, map[uint32]keys.Key{
		1: keys.Escape, 14: keys.Backspace, 15: keys.Tab, 28: keys.Enter, 29: keys.LCtrl,
		42: keys.LShift, 54: keys.RShift, 56: keys.LAlt, 58: keys.CapsLock,
		59: keys.F1, 60: keys.F2, 61: keys.F3, 62: keys.F4, 63: keys.F5, 64: keys.F6,
		65: keys.F7, 66: keys.F8, 67: keys.F9, 68: keys.F10, 69: keys.NumLock, 70: keys.ScrollLock,
		87: keys.F11, 88: keys.F12, 96: keys.Enter, 97: keys.RCtrl, 100: keys.RAlt,
		102: keys.Home, 103: keys.Up, 104: keys.PageUp, 105: keys.Left, 106: keys.Right,
		107: keys.End, 108: keys.Down, 109: keys.PageDown, 110: keys.Insert, 111: keys.Delete,
		119: keys.Pause, 125: keys.LMeta, 126: keys.RMeta,
	})
	return m
}()

var linuxCodes = strokeCodes()

type fakeKey struct {
	code int
	down bool
}

// XRecord не отличает наши события от пользовательских, поэтому бэкенд помнит
// отправленные через XTest нажатия и отмечает их по порядку прихода.
type Backend struct {
	mu       sync.Mutex
	expected []fakeKey
}

var (
	current *Backend
	handler func(keys.Event) bool // вызывается только из потока XRecord
)

func New() (*Backend, error) {
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return nil, errors.New("Wayland не поддерживается: войдите в сеанс X11 (Xorg)")
	}
	if C.lsOpen() == 0 {
		return nil, errors.New("не удалось подключиться к X-серверу")
	}
	current = &Backend{}
	return current, nil
}

// Run слушает клавиатуру через XRecord. Поглощать события XRecord не умеет.
func (b *Backend) Run(handle func(keys.Event) bool) error {
	handler = handle
	if r := C.lsRunRecord(); r != 0 {
		return fmt.Errorf("XRecord: ошибка %d", int(r))
	}
	return nil
}

func (b *Backend) SwitchLayout() error {
	if C.lsSwitchLayout() == 0 {
		return errors.New("не удалось переключить раскладку")
	}
	return nil
}

func (b *Backend) Send(strokes []keys.Stroke) error {
	shift := scanLShift + evdevOffset
	var seq []fakeKey
	for _, s := range strokes {
		code, ok := linuxCodes[s.Key]
		if !ok {
			continue
		}
		c := int(code) + evdevOffset
		if s.Shift {
			seq = append(seq, fakeKey{shift, true})
		}
		seq = append(seq, fakeKey{c, true}, fakeKey{c, false})
		if s.Shift {
			seq = append(seq, fakeKey{shift, false})
		}
	}
	b.play(seq)
	return nil
}

func (b *Backend) injected(code int, down bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.expected) > 0 && b.expected[0] == (fakeKey{code, down}) {
		b.expected = b.expected[1:]
		return true
	}
	return false
}

//export goHandleEvent
func goHandleEvent(typ, code C.int) {
	const keyPress, buttonPress = 2, 4
	if typ == buttonPress {
		if code <= 3 { // 4–7 — колесо мыши
			handler(keys.Event{Key: keys.Mouse, Down: true})
		}
		return
	}
	k, ok := linuxKeys[uint32(code)-evdevOffset]
	if !ok {
		k = keys.Other
	}
	down := typ == keyPress
	handler(keys.Event{Key: k, Down: down, Injected: current.injected(int(code), down)})
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// maxGroups — XKB поддерживает не больше 4 раскладок (групп).
const maxGroups = 4

// play отправляет нажатия через XTest и запоминает их, чтобы хук распознал свои события.
func (b *Backend) play(seq []fakeKey) {
	b.mu.Lock()
	b.expected = slices.Clone(seq)
	b.mu.Unlock()
	for _, f := range seq {
		C.lsFakeKey(C.int(f.code), cbool(f.down))
	}
	C.lsFlush()
}

func (b *Backend) Layouts() ([]keys.Layout, int, error) {
	var codes []C.int
	var ks []keys.Key
	for code, k := range scanKeys {
		codes = append(codes, C.int(code+evdevOffset))
		ks = append(ks, k)
	}
	out := make([]C.uint32_t, maxGroups*len(codes)*2)
	var cur C.int
	n := int(C.lsLayouts(&codes[0], C.int(len(codes)), &out[0], maxGroups, &cur))
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
	code, ok := linuxCodes[k]
	if !ok {
		return fmt.Errorf("нет кода клавиши для %v", k)
	}
	ctrl, c := scanLCtrl+evdevOffset, int(code)+evdevOffset
	b.play([]fakeKey{{ctrl, true}, {c, true}, {c, false}, {ctrl, false}})
	return nil
}

// ActiveApp возвращает имя исполняемого файла процесса активного окна (по _NET_WM_PID).
// Вызывается только из потока XRecord: у него своё соединение с X-сервером.
func (b *Backend) ActiveApp() string {
	pid := int(C.lsActivePID())
	if pid <= 0 {
		return ""
	}
	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		return filepath.Base(exe)
	}
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(comm))
}
