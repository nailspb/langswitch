//go:build windows

package clipboard

import (
	"bytes"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard            = user32.NewProc("OpenClipboard")
	procCloseClipboard           = user32.NewProc("CloseClipboard")
	procEmptyClipboard           = user32.NewProc("EmptyClipboard")
	procGetClipboardData         = user32.NewProc("GetClipboardData")
	procSetClipboardData         = user32.NewProc("SetClipboardData")
	procEnumClipboardFormats     = user32.NewProc("EnumClipboardFormats")
	procRegisterClipboardFormatW = user32.NewProc("RegisterClipboardFormatW")
	procGlobalAlloc              = kernel32.NewProc("GlobalAlloc")
	procGlobalFree               = kernel32.NewProc("GlobalFree")
	procGlobalLock               = kernel32.NewProc("GlobalLock")
	procGlobalUnlock             = kernel32.NewProc("GlobalUnlock")
	procGlobalSize               = kernel32.NewProc("GlobalSize")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// historyFormats — пометки, которые исключают содержимое из журнала буфера обмена (Win+V)
// и облачной синхронизации.
var historyFormats = sync.OnceValue(func() []uintptr {
	var formats []uintptr
	for _, name := range []string{
		"ExcludeClipboardContentFromMonitorProcessing",
		"CanIncludeInClipboardHistory",
		"CanUploadToCloudClipboard",
	} {
		p, _ := windows.UTF16PtrFromString(name)
		if f, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(p))); f != 0 {
			formats = append(formats, f)
		}
	}
	return formats
})

type Clipboard struct{}

func New() Clipboard { return Clipboard{} }

func (Clipboard) Text() string {
	var s string
	withClipboard(func() {
		h, _, _ := procGetClipboardData.Call(cfUnicodeText)
		if h == 0 {
			return
		}
		p, _, _ := procGlobalLock.Call(h)
		if p == 0 {
			return
		}
		defer procGlobalUnlock.Call(h)
		s = windows.UTF16PtrToString((*uint16)(unsafe.Pointer(p)))
	})
	return s
}

// SetText кладёт текст в буфер обмена, не добавляя его в журнал Win+V.
func (Clipboard) SetText(s string) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return // строка содержит NUL
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)
	withClipboard(func() {
		procEmptyClipboard.Call()
		set(cfUnicodeText, data)
		hideFromHistory()
	})
}

type entry struct {
	format uintptr
	data   []byte
}

// Save копирует содержимое буфера во всех форматах; restore возвращает его.
func (Clipboard) Save() (restore func()) {
	var saved []entry
	withClipboard(func() {
		for f, _, _ := procEnumClipboardFormats.Call(0); f != 0; f, _, _ = procEnumClipboardFormats.Call(f) {
			if skipFormat(f) {
				continue
			}
			if data, ok := get(f); ok {
				saved = append(saved, entry{f, data})
			}
		}
	})
	return func() {
		withClipboard(func() {
			procEmptyClipboard.Call()
			for _, e := range saved {
				set(e.format, e.data)
			}
			hideFromHistory() // содержимое уже есть в журнале — не дублируем
		})
	}
}

// skipFormat отсеивает форматы, которые хранятся не в глобальной памяти (GDI-объекты и т.п.):
// байтами их не скопировать, а Windows восстановит их сама из других форматов (например, CF_DIB).
func skipFormat(f uintptr) bool {
	switch f {
	case 2, 3, 9, 14, // CF_BITMAP, CF_METAFILEPICT, CF_PALETTE, CF_ENHMETAFILE
		0x80, 0x82, 0x83, 0x8E: // CF_OWNERDISPLAY, CF_DSPBITMAP, CF_DSPMETAFILEPICT, CF_DSPENHMETAFILE
		return true
	}
	return f >= 0x200 && f <= 0x3FF // CF_PRIVATEFIRST..CF_GDIOBJLAST
}

func get(f uintptr) ([]byte, bool) {
	h, _, _ := procGetClipboardData.Call(f)
	if h == 0 {
		return nil, false
	}
	size, _, _ := procGlobalSize.Call(h)
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return nil, false
	}
	defer procGlobalUnlock.Call(h)
	return bytes.Clone(unsafe.Slice((*byte)(unsafe.Pointer(p)), size)), true
}

func set(f uintptr, data []byte) {
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(max(len(data), 1)))
	if h == 0 {
		return
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
	procGlobalUnlock.Call(h)
	if r, _, _ := procSetClipboardData.Call(f, h); r == 0 {
		procGlobalFree.Call(h) // при успехе памятью владеет система
	}
}

func hideFromHistory() {
	zero := []byte{0, 0, 0, 0}
	for _, f := range historyFormats() {
		set(f, zero)
	}
}

// withClipboard открывает буфер обмена (повторяя попытки, пока он занят другой программой)
// и выполняет fn.
func withClipboard(fn func()) {
	// OpenClipboard и CloseClipboard должны вызываться из одного потока.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for range 20 {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			defer procCloseClipboard.Call()
			fn()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
