package engine

import (
	"strings"
	"unicode"

	"langswitch/internal/keys"
)

// translate переводит текст из раскладки, в которой он набран, в следующую за ней.
// Раскладка-источник — та, где есть больше всего букв текста; при равенстве — текущая.
// Возвращает новый текст и индекс раскладки, в которую он переведён; ok=false — менять нечего.
func translate(text string, layouts []keys.Layout, current int) (out string, target int, ok bool) {
	if len(layouts) < 2 {
		return "", 0, false
	}
	reverse := make([]map[rune]keys.Stroke, len(layouts))
	for i, l := range layouts {
		reverse[i] = reverseLayout(l)
	}

	source, best := 0, -1
	for i, rev := range reverse {
		score := 0
		for _, r := range text {
			if _, ok := rev[r]; ok && unicode.IsLetter(r) {
				score++
			}
		}
		if score > best || (score == best && i == current) {
			source, best = i, score
		}
	}

	target = (source + 1) % len(layouts)
	var b strings.Builder
	for _, r := range text {
		if s, ok := reverse[source][r]; ok {
			if t, ok := layouts[target][s]; ok {
				r = t
			}
		}
		b.WriteRune(r)
	}
	out = b.String()
	return out, target, out != text
}

// reverseLayout строит таблицу «символ → клавиша».
func reverseLayout(l keys.Layout) map[rune]keys.Stroke {
	m := make(map[rune]keys.Stroke, len(l))
	for s, r := range l {
		if old, ok := m[r]; !ok || strokeLess(s, old) {
			m[r] = s
		}
	}
	return m
}

// strokeLess задаёт однозначный выбор, если символ есть на нескольких клавишах.
func strokeLess(a, b keys.Stroke) bool {
	if a.Shift != b.Shift {
		return !a.Shift
	}
	return a.Key < b.Key
}
