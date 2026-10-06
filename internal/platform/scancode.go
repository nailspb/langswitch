// Package platform реализует engine.Backend для Windows, macOS и Linux (X11).
package platform

import "langswitch/internal/keys"

// scanKeys — печатные клавиши по скан-кодам Set 1. Для основного блока клавиатуры
// они совпадают с кодами evdev в Linux.
var scanKeys = map[uint32]keys.Key{
	0x02: keys.D1, 0x03: keys.D2, 0x04: keys.D3, 0x05: keys.D4, 0x06: keys.D5,
	0x07: keys.D6, 0x08: keys.D7, 0x09: keys.D8, 0x0A: keys.D9, 0x0B: keys.D0,
	0x0C: keys.Minus, 0x0D: keys.Equal,
	0x10: keys.Q, 0x11: keys.W, 0x12: keys.E, 0x13: keys.R, 0x14: keys.T,
	0x15: keys.Y, 0x16: keys.U, 0x17: keys.I, 0x18: keys.O, 0x19: keys.P,
	0x1A: keys.LBracket, 0x1B: keys.RBracket,
	0x1E: keys.A, 0x1F: keys.S, 0x20: keys.D, 0x21: keys.F, 0x22: keys.G,
	0x23: keys.H, 0x24: keys.J, 0x25: keys.K, 0x26: keys.L,
	0x27: keys.Semicolon, 0x28: keys.Quote, 0x29: keys.Grave, 0x2B: keys.Backslash,
	0x2C: keys.Z, 0x2D: keys.X, 0x2E: keys.C, 0x2F: keys.V, 0x30: keys.B,
	0x31: keys.N, 0x32: keys.M, 0x33: keys.Comma, 0x34: keys.Period, 0x35: keys.Slash,
	0x39: keys.Space, 0x56: keys.IntlBackslash,
}

const (
	scanBackspace = 0x0E
	scanLShift    = 0x2A
)

// strokeCodes — обратная таблица scanKeys для ввода, плюс Backspace.
func strokeCodes() map[keys.Key]uint32 {
	m := make(map[keys.Key]uint32, len(scanKeys)+1)
	for code, k := range scanKeys {
		m[k] = code
	}
	m[keys.Backspace] = scanBackspace
	return m
}

const scanLCtrl = 0x1D

// buildLayouts собирает раскладки из плоской таблицы символов
// [раскладка][клавиша][shift]; char(i) возвращает 0, если символа нет.
func buildLayouts(n int, ks []keys.Key, char func(i int) rune) []keys.Layout {
	layouts := make([]keys.Layout, n)
	for l := range n {
		m := keys.Layout{}
		for i, k := range ks {
			for s, shift := range []bool{false, true} {
				if r := char((l*len(ks)+i)*2 + s); r != 0 {
					m[keys.Stroke{Key: k, Shift: shift}] = r
				}
			}
		}
		layouts[l] = m
	}
	return layouts
}
