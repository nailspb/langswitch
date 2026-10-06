// Package keys описывает клавиши независимо от платформы.
package keys

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Key — физическая клавиша (по положению на клавиатуре, а не по символу).
type Key uint8

const (
	None Key = iota

	// Модификаторы с учётом стороны.
	LShift
	RShift
	LCtrl
	RCtrl
	LAlt
	RAlt
	LMeta
	RMeta

	// Модификаторы без учёта стороны — используются только в настройках.
	Shift
	Ctrl
	Alt
	Meta

	// Печатные клавиши.
	A
	B
	C
	D
	E
	F
	G
	H
	I
	J
	K
	L
	M
	N
	O
	P
	Q
	R
	S
	T
	U
	V
	W
	X
	Y
	Z
	D0
	D1
	D2
	D3
	D4
	D5
	D6
	D7
	D8
	D9
	Grave
	Minus
	Equal
	LBracket
	RBracket
	Backslash
	Semicolon
	Quote
	Comma
	Period
	Slash
	IntlBackslash

	Space
	Backspace
	Enter
	Tab
	Escape
	CapsLock
	Left
	Right
	Up
	Down
	Home
	End
	PageUp
	PageDown
	Insert
	Delete
	F1
	F2
	F3
	F4
	F5
	F6
	F7
	F8
	F9
	F10
	F11
	F12
	Pause
	ScrollLock
	NumLock

	Other // любая неизвестная клавиша
	Mouse // нажатие кнопки мыши
)

var names = func() [Mouse + 1]string {
	n := [Mouse + 1]string{
		LShift: "LShift", RShift: "RShift", LCtrl: "LCtrl", RCtrl: "RCtrl",
		LAlt: "LAlt", RAlt: "RAlt", LMeta: "LMeta", RMeta: "RMeta",
		Shift: "Shift", Ctrl: "Ctrl", Alt: "Alt", Meta: "Meta",
		Grave: "`", Minus: "-", Equal: "=", LBracket: "[", RBracket: "]", Backslash: `\`,
		Semicolon: ";", Quote: "'", Comma: ",", Period: ".", Slash: "/", IntlBackslash: "IntlBackslash",
		Space: "Space", Backspace: "Backspace", Enter: "Enter", Tab: "Tab", Escape: "Esc",
		CapsLock: "CapsLock", Left: "Left", Right: "Right", Up: "Up", Down: "Down",
		Home: "Home", End: "End", PageUp: "PageUp", PageDown: "PageDown",
		Insert: "Insert", Delete: "Delete", Pause: "Pause", ScrollLock: "ScrollLock", NumLock: "NumLock",
		Other: "Other", Mouse: "Mouse",
	}
	for i := range 26 {
		n[A+Key(i)] = string(rune('A' + i))
	}
	for i := range 10 {
		n[D0+Key(i)] = strconv.Itoa(i)
	}
	for i := range 12 {
		n[F1+Key(i)] = "F" + strconv.Itoa(i+1)
	}
	return n
}()

func (k Key) String() string {
	if int(k) < len(names) && names[k] != "" {
		return names[k]
	}
	return fmt.Sprintf("Key(%d)", k)
}

// Parse возвращает клавишу по имени без учёта регистра.
func Parse(s string) (Key, bool) {
	for k := LShift; k < Other; k++ {
		if strings.EqualFold(names[k], s) {
			return k, true
		}
	}
	return None, false
}

// IsModifier сообщает, является ли клавиша модификатором.
func (k Key) IsModifier() bool { return k >= LShift && k <= Meta }

// IsPrintable сообщает, вводит ли клавиша символ, зависящий от раскладки.
func (k Key) IsPrintable() bool { return k >= A && k <= IntlBackslash }

// Generic возвращает модификатор без учёта стороны (LShift → Shift).
func (k Key) Generic() Key {
	switch k {
	case LShift, RShift:
		return Shift
	case LCtrl, RCtrl:
		return Ctrl
	case LAlt, RAlt:
		return Alt
	case LMeta, RMeta:
		return Meta
	}
	return k
}

// Matches сообщает, соответствует ли нажатая клавиша actual клавише из настроек k.
func (k Key) Matches(actual Key) bool { return k == actual || k == actual.Generic() }

func (k Key) isGeneric() bool { return k >= Shift && k <= Meta }

// Event — нажатие или отпускание клавиши, полученное от системного хука.
type Event struct {
	Key      Key
	Down     bool
	Injected bool // событие сгенерировано программно, в том числе нами
}

// Stroke — нажатие клавиши для повторного ввода.
type Stroke struct {
	Key   Key
	Shift bool
}

// Hotkey — сочетание клавиш: модификаторы и не более одной обычной клавиши.
type Hotkey []Key

// ParseHotkey разбирает сочетание вида "Ctrl+Shift" или "CapsLock".
func ParseHotkey(s string) (Hotkey, error) {
	var h Hotkey
	plain := 0
	for part := range strings.SplitSeq(s, "+") {
		k, ok := Parse(strings.TrimSpace(part))
		if !ok {
			return nil, fmt.Errorf("неизвестная клавиша %q", part)
		}
		if !k.IsModifier() {
			plain++
		}
		h = append(h, k)
	}
	if plain > 1 {
		return nil, errors.New("в сочетании может быть только одна обычная клавиша")
	}
	return h, nil
}

func (h Hotkey) String() string {
	parts := make([]string, len(h))
	for i, k := range h {
		parts[i] = k.String()
	}
	return strings.Join(parts, "+")
}

// Trigger возвращает обычную (не модификатор) клавишу сочетания или None.
func (h Hotkey) Trigger() Key {
	for _, k := range h {
		if !k.IsModifier() {
			return k
		}
	}
	return None
}

// Match сообщает, совпадает ли набор нажатых клавиш с сочетанием.
func (h Hotkey) Match(pressed []Key) bool {
	if len(h) != len(pressed) {
		return false
	}
	used := make([]bool, len(pressed))
	// Сначала точные клавиши, затем обобщённые: иначе Shift может «занять» LShift,
	// нужный другой части сочетания.
	for _, generic := range []bool{false, true} {
		for _, want := range h {
			if want.isGeneric() != generic {
				continue
			}
			found := false
			for i, got := range pressed {
				if !used[i] && want.Matches(got) {
					used[i], found = true, true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

// Layout — символы, которые вводят клавиши в раскладке.
type Layout map[Stroke]rune
