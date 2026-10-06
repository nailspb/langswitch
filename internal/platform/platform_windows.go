//go:build windows

package platform

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"unsafe"

	"golang.org/x/sys/windows"

	"langswitch/internal/keys"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookExW        = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx           = user32.NewProc("CallNextHookEx")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procSendInput                = user32.NewProc("SendInput")
	procGetAsyncKeyState         = user32.NewProc("GetAsyncKeyState")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	procGetKeyboardLayoutList    = user32.NewProc("GetKeyboardLayoutList")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procGetGUIThreadInfo         = user32.NewProc("GetGUIThreadInfo")
	procMapVirtualKeyExW         = user32.NewProc("MapVirtualKeyExW")
	procToUnicodeEx              = user32.NewProc("ToUnicodeEx")
)

const (
	hcAction                 = 0
	whKeyboardLL             = 13
	whMouseLL                = 14
	wmInputLangChangeRequest = 0x0050
	wmKeyDown                = 0x0100
	wmSysKeyDown             = 0x0104
	wmLButtonDown            = 0x0201
	wmRButtonDown            = 0x0204
	wmMButtonDown            = 0x0207
	llkhfExtended            = 0x01
	llkhfInjected            = 0x10
	llmhfInjected            = 0x01
	inputKeyboard            = 1
	keyeventfKeyUp           = 0x0002
	keyeventfScancode        = 0x0008
	mapvkVscToVk             = 1
	vkShift                  = 0x10
	vkLShift                 = 0xA0
	toUnicodeNoStateChange   = 0x4 // не трогать состояние мёртвых клавиш (Windows 10 1607+)
)

type kbdllhookstruct struct {
	vkCode    uint32
	scanCode  uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type msllhookstruct struct {
	pt        struct{ x, y int32 }
	mouseData uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type keybdInput struct {
	vk        uint16
	scan      uint16
	flags     uint32
	time      uint32
	extraInfo uintptr
}

// input соответствует INPUT; хвост дополняет union до размера MOUSEINPUT.
type input struct {
	typ uint32
	ki  keybdInput
	_   [8]byte
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

// vkKeys — непечатные клавиши по виртуальным кодам (от раскладки не зависят).
var vkKeys = map[uint32]keys.Key{
	0xA0: keys.LShift, 0xA1: keys.RShift, 0xA2: keys.LCtrl, 0xA3: keys.RCtrl,
	0xA4: keys.LAlt, 0xA5: keys.RAlt, 0x5B: keys.LMeta, 0x5C: keys.RMeta,
	0x08: keys.Backspace, 0x09: keys.Tab, 0x0D: keys.Enter, 0x1B: keys.Escape,
	0x14: keys.CapsLock, 0x13: keys.Pause, 0x91: keys.ScrollLock, 0x90: keys.NumLock,
	0x21: keys.PageUp, 0x22: keys.PageDown, 0x23: keys.End, 0x24: keys.Home,
	0x25: keys.Left, 0x26: keys.Up, 0x27: keys.Right, 0x28: keys.Down,
	0x2D: keys.Insert, 0x2E: keys.Delete,
	0x70: keys.F1, 0x71: keys.F2, 0x72: keys.F3, 0x73: keys.F4, 0x74: keys.F5, 0x75: keys.F6,
	0x76: keys.F7, 0x77: keys.F8, 0x78: keys.F9, 0x79: keys.F10, 0x7A: keys.F11, 0x7B: keys.F12,
}

var injectScan = strokeCodes()

type Backend struct {
	handle func(keys.Event) bool

	appWnd  uintptr // окно, для которого определено appName (используются только из потока хука)
	appName string
}

func New() (*Backend, error) { return &Backend{}, nil }

func (b *Backend) Run(handle func(keys.Event) bool) error {
	// Хуки вызываются в потоке, который их установил, через его цикл сообщений.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	b.handle = handle

	var mod windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &mod); err != nil {
		return fmt.Errorf("GetModuleHandleEx: %w", err)
	}
	if err := setHook(whKeyboardLL, b.keyboardProc, mod); err != nil {
		return err
	}
	if err := setHook(whMouseLL, b.mouseProc, mod); err != nil {
		return err
	}

	var m msg
	for {
		r, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(r) {
		case -1:
			return fmt.Errorf("GetMessage: %w", err)
		case 0:
			return nil
		}
	}
}

func setHook(id uintptr, proc func(code, wParam, lParam uintptr) uintptr, mod windows.Handle) error {
	h, _, err := procSetWindowsHookExW.Call(id, windows.NewCallback(proc), uintptr(mod), 0)
	if h == 0 {
		return fmt.Errorf("SetWindowsHookEx(%d): %w", id, err)
	}
	return nil
}

func (b *Backend) keyboardProc(code, wParam, lParam uintptr) uintptr {
	if int32(code) == hcAction {
		kb := (*kbdllhookstruct)(unsafe.Pointer(lParam))
		ev := keys.Event{
			Key:      keyOf(kb),
			Down:     wParam == wmKeyDown || wParam == wmSysKeyDown,
			Injected: kb.flags&llkhfInjected != 0,
		}
		if b.handle(ev) {
			return 1
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, code, wParam, lParam)
	return r
}

func (b *Backend) mouseProc(code, wParam, lParam uintptr) uintptr {
	if int32(code) == hcAction {
		switch wParam {
		case wmLButtonDown, wmRButtonDown, wmMButtonDown:
			ms := (*msllhookstruct)(unsafe.Pointer(lParam))
			b.handle(keys.Event{Key: keys.Mouse, Down: true, Injected: ms.flags&llmhfInjected != 0})
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, code, wParam, lParam)
	return r
}

func keyOf(kb *kbdllhookstruct) keys.Key {
	if k, ok := vkKeys[kb.vkCode]; ok {
		return k
	}
	if kb.flags&llkhfExtended == 0 {
		if k, ok := scanKeys[kb.scanCode]; ok {
			return k
		}
	}
	return keys.Other
}

// Pressed проверяет физическое состояние модификатора.
func (b *Backend) Pressed(k keys.Key) bool {
	for vk, key := range vkKeys {
		if key == k {
			r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
			return r&0x8000 != 0
		}
	}
	return true
}

// Send вводит клавиши скан-кодами, чтобы символы определялись текущей раскладкой окна.
func (b *Backend) Send(strokes []keys.Stroke) error {
	var in []input
	for _, s := range strokes {
		sc, ok := injectScan[s.Key]
		if !ok {
			continue
		}
		if s.Shift {
			in = append(in, scanInput(scanLShift, false))
		}
		in = append(in, scanInput(sc, false), scanInput(sc, true))
		if s.Shift {
			in = append(in, scanInput(scanLShift, true))
		}
	}
	return sendInputs(in)
}

func scanInput(sc uint32, up bool) input {
	flags := uint32(keyeventfScancode)
	if up {
		flags |= keyeventfKeyUp
	}
	return input{typ: inputKeyboard, ki: keybdInput{scan: uint16(sc), flags: flags}}
}

// SwitchLayout просит активное окно включить следующую раскладку из списка системы.
func (b *Backend) SwitchLayout() error {
	target, tid, err := foreground()
	if err != nil {
		return err
	}
	list, err := layoutList()
	if err != nil {
		return err
	}
	cur, _, _ := procGetKeyboardLayout.Call(tid)
	return requestLayout(target, list[(slices.Index(list, cur)+1)%len(list)])
}

type guiThreadInfo struct {
	size          uint32
	flags         uint32
	hwndActive    uintptr
	hwndFocus     uintptr
	hwndCapture   uintptr
	hwndMenuOwner uintptr
	hwndMoveSize  uintptr
	hwndCaret     uintptr
	rcCaret       struct{ left, top, right, bottom int32 }
}

func sendInputs(in []input) error {
	if len(in) == 0 {
		return nil
	}
	n, _, err := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(n) != len(in) {
		return fmt.Errorf("SendInput: %w", err)
	}
	return nil
}

// foreground возвращает окно, которому отправлять запрос смены раскладки, и его поток.
// Запрос уходит окну с фокусом ввода: диалоги (например, «Выполнить») игнорируют его,
// если послать главному окну.
func foreground() (target, tid uintptr, err error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return 0, 0, errors.New("нет активного окна")
	}
	tid, _, _ = procGetWindowThreadProcessId.Call(hwnd, 0)
	target = hwnd
	gti := guiThreadInfo{size: uint32(unsafe.Sizeof(guiThreadInfo{}))}
	if r, _, _ := procGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gti))); r != 0 && gti.hwndFocus != 0 {
		target = gti.hwndFocus
	}
	return target, tid, nil
}

// layoutList возвращает раскладки системы в порядке переключения.
func layoutList() ([]uintptr, error) {
	n, _, _ := procGetKeyboardLayoutList.Call(0, 0)
	list := make([]uintptr, n)
	if n > 0 {
		n, _, _ = procGetKeyboardLayoutList.Call(n, uintptr(unsafe.Pointer(&list[0])))
		list = list[:n]
	}
	if len(list) == 0 {
		return nil, errors.New("не найдено ни одной раскладки")
	}
	return list, nil
}

func requestLayout(target, hkl uintptr) error {
	if r, _, err := procPostMessageW.Call(target, wmInputLangChangeRequest, 0, hkl); r == 0 {
		return fmt.Errorf("PostMessage: %w", err)
	}
	return nil
}

func (b *Backend) Layouts() ([]keys.Layout, int, error) {
	_, tid, err := foreground()
	if err != nil {
		return nil, 0, err
	}
	list, err := layoutList()
	if err != nil {
		return nil, 0, err
	}
	cur, _, _ := procGetKeyboardLayout.Call(tid)
	layouts := make([]keys.Layout, len(list))
	for i, hkl := range list {
		layouts[i] = keymap(hkl)
	}
	return layouts, slices.Index(list, cur), nil
}

// keymap возвращает символы печатных клавиш в раскладке hkl.
func keymap(hkl uintptr) keys.Layout {
	m := keys.Layout{}
	var state [256]byte
	var buf [4]uint16
	for sc, k := range scanKeys {
		vk, _, _ := procMapVirtualKeyExW.Call(uintptr(sc), mapvkVscToVk, hkl)
		if vk == 0 {
			continue
		}
		for _, shift := range []bool{false, true} {
			state[vkShift], state[vkLShift] = 0, 0
			if shift {
				state[vkShift], state[vkLShift] = 0x80, 0x80
			}
			n, _, _ := procToUnicodeEx.Call(vk, uintptr(sc), uintptr(unsafe.Pointer(&state[0])),
				uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), toUnicodeNoStateChange, hkl)
			if int32(n) == 1 {
				m[keys.Stroke{Key: k, Shift: shift}] = rune(buf[0])
			}
		}
	}
	return m
}

func (b *Backend) SetLayout(i int) error {
	target, _, err := foreground()
	if err != nil {
		return err
	}
	list, err := layoutList()
	if err != nil {
		return err
	}
	if i < 0 || i >= len(list) {
		return fmt.Errorf("нет раскладки с индексом %d", i)
	}
	return requestLayout(target, list[i])
}

func (b *Backend) SendShortcut(k keys.Key) error {
	sc, ok := injectScan[k]
	if !ok {
		return fmt.Errorf("нет скан-кода для %v", k)
	}
	return sendInputs([]input{
		scanInput(scanLCtrl, false), scanInput(sc, false), scanInput(sc, true), scanInput(scanLCtrl, true),
	})
}

// ActiveApp возвращает имя исполняемого файла активного окна, например "notepad.exe".
// Имя запоминается для окна, чтобы не открывать процесс на каждое нажатие.
func (b *Backend) ActiveApp() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}
	if hwnd != b.appWnd {
		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		b.appWnd, b.appName = hwnd, processName(pid)
	}
	return b.appName
}

func processName(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}
