//go:build darwin

package clipboard

/*
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
char *lsClipText(void);
void  lsClipSetText(const char *s);
void *lsClipSave(void);
void  lsClipRestore(void *saved);
*/
import "C"

import "unsafe"

type Clipboard struct{}

func New() Clipboard { return Clipboard{} }

func (Clipboard) Text() string {
	p := C.lsClipText()
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

// SetText кладёт текст в буфер обмена с пометкой «временные данные» для менеджеров истории.
func (Clipboard) SetText(s string) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	C.lsClipSetText(cs)
}

// Save копирует содержимое буфера во всех форматах; restore возвращает его
// и должна быть вызвана ровно один раз (освобождает сохранённые данные).
func (Clipboard) Save() (restore func()) {
	saved := C.lsClipSave()
	return func() { C.lsClipRestore(saved) }
}
