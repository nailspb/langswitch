// Package engine распознаёт горячие клавиши и выполняет переключение и конвертацию.
package engine

import (
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"langswitch/internal/keys"
	"langswitch/internal/sound"
)

// Backend — платформенная часть: перехват клавиатуры, раскладки и ввод.
type Backend interface {
	// Run устанавливает хук и блокируется. handle вызывается на каждое событие
	// из одного потока; true — поглотить событие (если платформа это умеет).
	Run(handle func(keys.Event) bool) error
	// SwitchLayout включает следующую раскладку в активном окне.
	SwitchLayout() error
	// Send вводит нажатия клавиш.
	Send(strokes []keys.Stroke) error
	// Layouts возвращает символы клавиш всех раскладок системы и индекс текущей (-1, если не найдена).
	Layouts() (layouts []keys.Layout, current int, err error)
	// SetLayout включает раскладку по индексу из Layouts.
	SetLayout(i int) error
	// SendShortcut нажимает k с основным модификатором системы (Ctrl, на macOS — Cmd).
	SendShortcut(k keys.Key) error
	// ActiveApp возвращает имя исполняемого файла активного приложения или "".
	// Вызывается из потока хука на каждое нажатие, поэтому должен быть быстрым.
	ActiveApp() string
}

// stater — бэкенд, умеющий проверить физическое состояние клавиши. Нужен, чтобы
// не «залипали» модификаторы, отпускание которых система не прислала (например, после Win+L).
type stater interface {
	Pressed(k keys.Key) bool
}

type Settings struct {
	Switch   []keys.Hotkey // сочетания переключения языка
	Convert  []keys.Key    // клавиши конвертации последнего слова двойным нажатием
	Interval time.Duration // максимальное время двойного нажатия
	Sound    bool          // щелчок при переключении раскладки

	WordOn       bool       // конвертация последнего слова
	SelectionOn  bool       // конвертация выделенного текста
	PhraseOn     bool       // конвертация всей фразы
	PhraseTriple bool       // фраза — тройным нажатием клавиши из Convert, иначе двойным нажатием клавиши из Phrase
	Phrase       []keys.Key // клавиши конвертации фразы
	Excluded     []string   // приложения, в которых программа не работает
}

const (
	maxBuffer   = 256
	layoutDelay = 50 * time.Millisecond  // время на применение раскладки перед вводом
	copyTimeout = 300 * time.Millisecond // ожидание выделенного текста в буфере обмена после Ctrl+C
	pasteDelay  = 300 * time.Millisecond // приложение читает буфер при вставке асинхронно
)

type Engine struct {
	backend  Backend
	settings atomic.Pointer[Settings]
	capture  atomic.Pointer[func(keys.Hotkey)]
	actions  chan action

	// Поля ниже используются только из потока хука.
	held       []keys.Key        // нажатые сейчас клавиши
	swallowed  map[keys.Key]bool // клавиши, нажатие которых поглощено, — поглощаем и отпускание
	chord      []keys.Key        // модификаторы, нажатые с момента, когда все были отпущены
	chordDirty bool              // в аккорде участвовало что-то кроме модификаторов
	captured   []keys.Key        // клавиши, нажатые в режиме записи сочетания
	buffer     []keys.Stroke     // набранный текст в виде нажатий
	taps       int
	tapStart   time.Time
	tapDown    bool
	tapKey     keys.Key // клавиша из настроек, двойное нажатие которой отслеживается

	clipboard Clipboard // используется только из рабочей горутины

	appCapture atomic.Pointer[func(string)]
	self       string    // имя собственного исполняемого файла
	lastTap    time.Time // отпускание предыдущего нажатия клавиши конвертации
	tapPhrase  bool      // tapKey — клавиша конвертации фразы
	wordDone   bool      // двойное нажатие сконвертировало слово (раскладка уже переключена)
}

func New(b Backend, c Clipboard, s Settings) *Engine {
	e := &Engine{
		backend:   b,
		actions:   make(chan action, 2), // слово и сразу за ним фраза при тройном нажатии
		swallowed: map[keys.Key]bool{},
		clipboard: c,
	}
	if exe, err := os.Executable(); err == nil {
		e.self = filepath.Base(exe)
	}
	e.SetSettings(s)
	return e
}

func (e *Engine) SetSettings(s Settings) { e.settings.Store(&s) }

// Capture записывает следующее сочетание клавиш вместо его обработки.
func (e *Engine) Capture(done func(keys.Hotkey)) { e.capture.Store(&done) }

func (e *Engine) CancelCapture() { e.capture.Store(nil) }

// Run запускает обработку и блокируется до ошибки хука.
func (e *Engine) Run() error {
	go e.worker()
	return e.backend.Run(e.handle)
}

func (e *Engine) handle(ev keys.Event) bool {
	if ev.Injected {
		return false
	}
	if ev.Down && ev.Key != keys.Mouse && e.skipApp() {
		e.resetInput()
		return false
	}
	if ev.Key == keys.Mouse {
		e.resetInput()
		return false
	}
	if !ev.Down {
		e.held = slices.DeleteFunc(e.held, func(k keys.Key) bool { return k == ev.Key })
		return e.keyUp(ev.Key)
	}
	if st, ok := e.backend.(stater); ok {
		e.held = slices.DeleteFunc(e.held, func(k keys.Key) bool {
			return k != ev.Key && k.IsModifier() && !st.Pressed(k)
		})
	}
	repeat := slices.Contains(e.held, ev.Key)
	if !repeat {
		e.held = append(e.held, ev.Key)
	}
	return e.keyDown(ev.Key, repeat)
}

func (e *Engine) keyDown(k keys.Key, repeat bool) bool {
	s := e.settings.Load()
	if e.capture.Load() != nil {
		if !repeat && !slices.Contains(e.captured, k) {
			e.captured = append(e.captured, k)
		}
		return e.swallow(k)
	}
	e.captured = e.captured[:0]
	if !repeat {
		e.tap(k, true, s)
	}

	if k.IsModifier() {
		if !repeat {
			if e.modifiersHeld() == 1 {
				e.chord, e.chordDirty = e.chord[:0], false
			}
			e.chord = append(e.chord, k)
		}
		return false
	}

	e.chordDirty = true
	if e.swallowed[k] {
		return true // автоповтор поглощённой клавиши
	}
	if slices.ContainsFunc(s.Switch, func(h keys.Hotkey) bool {
		return h.Trigger().Matches(k) && h.Match(e.withModifiers(k))
	}) {
		e.request(action{})
		return e.swallow(k)
	}
	e.record(k)
	return false
}

func (e *Engine) keyUp(k keys.Key) bool {
	s := e.settings.Load()
	swallowed := e.swallowed[k]
	delete(e.swallowed, k)

	if fn := e.capture.Load(); fn != nil {
		if len(e.held) == 0 && len(e.captured) > 0 && e.capture.CompareAndSwap(fn, nil) {
			go (*fn)(hotkeyFrom(e.captured))
			e.captured = e.captured[:0]
		}
		return swallowed
	}

	e.tap(k, false, s)
	// Сочетание только из модификаторов срабатывает при отпускании всех клавиш,
	// если между ними не нажималось ничего другого.
	if k.IsModifier() && e.modifiersHeld() == 0 && !e.chordDirty &&
		slices.ContainsFunc(s.Switch, func(h keys.Hotkey) bool { return h.Trigger() == keys.None && h.Match(e.chord) }) {
		e.chordDirty = true
		e.request(action{})
	}
	return swallowed
}

// tap распознаёт двойное и тройное нажатие клавиши конвертации без других клавиш между нажатиями.
// Между нажатиями (и в каждом удержании) должно пройти не больше s.Interval.
func (e *Engine) tap(k keys.Key, down bool, s *Settings) {
	tk, phrase := s.tapKey(k)
	now := time.Now()
	if down {
		if tk == keys.None || len(e.held) > 1 {
			e.taps, e.tapDown = 0, false
			return
		}
		if tk != e.tapKey || now.Sub(e.lastTap) > s.Interval {
			e.taps, e.tapKey, e.tapPhrase = 0, tk, phrase
		}
		e.tapStart, e.tapDown = now, true
		return
	}
	if !e.tapDown || tk != e.tapKey {
		return
	}
	e.tapDown = false
	if now.Sub(e.tapStart) > s.Interval { // долгое удержание — не нажатие
		e.taps = 0
		return
	}
	e.taps++
	e.lastTap = now
	switch {
	case e.tapPhrase:
		if e.taps == 2 {
			e.taps = 0
			e.convertPhrase(true)
		}
	case e.taps == 2:
		e.wordDone = e.convertWord(s)
		if !s.PhraseOn || !s.PhraseTriple {
			e.taps = 0
		}
	case e.taps == 3:
		e.taps = 0
		// Если второе нажатие сконвертировало слово, раскладка уже переключена —
		// остаётся перепечатать в ней всю фразу.
		e.convertPhrase(!e.wordDone)
	}
}

// record запоминает нажатие в буфере набранного текста.
func (e *Engine) record(k keys.Key) {
	shift, combo := false, false
	for _, m := range e.held {
		switch {
		case keys.Shift.Matches(m):
			shift = true
		case m.IsModifier():
			combo = true
		}
	}
	switch {
	case combo: // Ctrl+V и т.п. — текст мог измениться непредсказуемо
		e.buffer = e.buffer[:0]
	case k.IsPrintable() || k == keys.Space:
		e.buffer = append(e.buffer, keys.Stroke{Key: k, Shift: shift})
	case k == keys.Backspace:
		if n := len(e.buffer); n > 0 {
			e.buffer = e.buffer[:n-1]
		}
	case k == keys.CapsLock:
	default: // стрелки, Enter и т.п. — курсор ушёл
		e.buffer = e.buffer[:0]
	}
	if len(e.buffer) > maxBuffer {
		e.buffer = slices.Delete(e.buffer, 0, len(e.buffer)-maxBuffer)
	}
}

// lastWord возвращает последнее слово вместе с пробелами после него или nil.
func (e *Engine) lastWord() []keys.Stroke {
	i := len(e.buffer)
	for i > 0 && e.buffer[i-1].Key == keys.Space {
		i--
	}
	end := i
	for i > 0 && e.buffer[i-1].Key != keys.Space {
		i--
	}
	if i == end {
		return nil
	}
	return slices.Clone(e.buffer[i:])
}

func (e *Engine) resetInput() {
	e.buffer = e.buffer[:0]
	e.taps, e.tapDown = 0, false
	e.chordDirty = true
}

func (e *Engine) swallow(k keys.Key) bool {
	if k.IsModifier() {
		return false // модификаторы не поглощаем, чтобы не сломать их состояние в системе
	}
	e.swallowed[k] = true
	return true
}

func (e *Engine) modifiersHeld() int {
	n := 0
	for _, k := range e.held {
		if k.IsModifier() {
			n++
		}
	}
	return n
}

func (e *Engine) withModifiers(k keys.Key) []keys.Key {
	var pressed []keys.Key
	for _, h := range e.held {
		if h.IsModifier() {
			pressed = append(pressed, h)
		}
	}
	return append(pressed, k)
}

func (e *Engine) request(a action) {
	select {
	case e.actions <- a:
	default: // предыдущее действие ещё выполняется
	}
}

func (e *Engine) worker() {
	for a := range e.actions {
		var err error
		if a.selection {
			err = e.convertSelection()
		} else {
			err = e.convert(a.word, !a.keepLayout)
		}
		if err != nil {
			log.Printf("langswitch: %v", err)
		}
	}
}

// convert стирает слово, переключает раскладку (если switchLayout) и набирает те же клавиши заново.
// Буфер при этом не меняется, поэтому повторная конвертация возвращает исходный текст.
func (e *Engine) convert(word []keys.Stroke, switchLayout bool) error {
	if len(word) > 0 {
		erase := slices.Repeat([]keys.Stroke{{Key: keys.Backspace}}, len(word))
		if err := e.backend.Send(erase); err != nil {
			return err
		}
	}
	if switchLayout {
		if err := e.backend.SwitchLayout(); err != nil {
			return err
		}
		e.beep()
		if len(word) == 0 {
			return nil
		}
		time.Sleep(layoutDelay)
	}
	return e.backend.Send(word)
}

// hotkeyFrom строит сочетание из записанных клавиш: одиночная клавиша сохраняется
// как есть (RCtrl), в сочетаниях модификаторы обобщаются (Ctrl+Shift).
func hotkeyFrom(pressed []keys.Key) keys.Hotkey {
	if len(pressed) == 1 {
		return keys.Hotkey{pressed[0]}
	}
	var h keys.Hotkey
	plain := keys.None
	for _, k := range pressed {
		switch {
		case !k.IsModifier():
			plain = k
		case !slices.Contains(h, k.Generic()):
			h = append(h, k.Generic())
		}
	}
	if plain != keys.None {
		h = append(h, plain)
	}
	return h
}

// convertKey возвращает клавишу конвертации из настроек, которой соответствует k, или None.
func (s *Settings) convertKey(k keys.Key) keys.Key {
	for _, c := range s.Convert {
		if c.Matches(k) {
			return c
		}
	}
	return keys.None
}

// Clipboard — системный буфер обмена.
type Clipboard interface {
	Text() string
	// SetText кладёт временный текст; по возможности он не попадает в историю буфера обмена.
	SetText(s string)
	// Save запоминает содержимое буфера (по возможности во всех форматах);
	// restore возвращает его и вызывается ровно один раз.
	Save() (restore func())
}

// action — задание для рабочей горутины. Пустое — только переключить язык.
type action struct {
	word      []keys.Stroke // последнее слово для перепечатки
	selection bool          // сконвертировать выделенный текст

	keepLayout bool // не переключать раскладку: её уже переключила конвертация слова
}

// convertSelection копирует выделенный текст, переводит его в другую раскладку,
// вставляет обратно и включает эту раскладку. Без выделения просто переключает язык.
func (e *Engine) convertSelection() error {
	text, restore, err := e.copySelection()
	if err != nil {
		return err
	}
	defer restore()
	if text == "" {
		return e.convert(nil, true)
	}

	layouts, cur, err := e.backend.Layouts()
	if err != nil {
		return err
	}
	out, target, ok := translate(text, layouts, cur)
	if !ok {
		return e.convert(nil, true)
	}
	e.clipboard.SetText(out)
	if err := e.backend.SendShortcut(keys.V); err != nil {
		return err
	}
	time.Sleep(pasteDelay)
	if target != cur {
		if err := e.backend.SetLayout(target); err != nil {
			return err
		}
		e.beep()
	}
	return nil
}

// copySelection копирует выделенный текст через буфер обмена. restore возвращает
// прежнее содержимое буфера. Пустая строка — выделения нет.
func (e *Engine) copySelection() (text string, restore func(), err error) {
	const probe = "langswitch: проверка выделения"
	restore = e.clipboard.Save()

	e.clipboard.SetText(probe)
	if err := e.backend.SendShortcut(keys.C); err != nil {
		restore()
		return "", nil, err
	}
	for deadline := time.Now().Add(copyTimeout); time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		text = e.clipboard.Text()
		if text == probe {
			continue
		}
		// Ctrl+C без выделения в некоторых редакторах (VS Code, JetBrains) копирует
		// всю строку с переводом строки — это не выделение.
		if strings.HasSuffix(text, "\n") {
			text = ""
		}
		return text, restore, nil
	}
	return "", restore, nil
}

func (e *Engine) beep() {
	if e.settings.Load().Sound {
		sound.Play()
	}
}

// CaptureApp передаёт done имя приложения, в котором будет нажата следующая клавиша
// (нажатия в самой программе пропускаются).
func (e *Engine) CaptureApp(done func(app string)) { e.appCapture.Store(&done) }

func (e *Engine) CancelCaptureApp() { e.appCapture.Store(nil) }

// skipApp проверяет активное приложение: отдаёт его имя ожидающему CaptureApp
// и сообщает, исключено ли оно в настройках.
func (e *Engine) skipApp() bool {
	s := e.settings.Load()
	pick := e.appCapture.Load()
	if pick == nil && len(s.Excluded) == 0 {
		return false
	}
	app := e.backend.ActiveApp()
	if app == "" {
		return false
	}
	if pick != nil && !strings.EqualFold(app, e.self) && e.appCapture.CompareAndSwap(pick, nil) {
		go (*pick)(app)
	}
	return slices.ContainsFunc(s.Excluded, func(x string) bool { return strings.EqualFold(x, app) })
}

// convertWord конвертирует последнее слово, а если его нет — выделенный текст.
// Возвращает true, если запрошена конвертация слова.
func (e *Engine) convertWord(s *Settings) bool {
	// Если после последнего перемещения курсора набрано слово — конвертируем его.
	// Иначе текст могли выделить (мышью, Shift+стрелками, Ctrl+A) — пробуем выделение.
	if word := e.lastWord(); word != nil {
		if s.WordOn {
			e.request(action{word: word})
		}
		return s.WordOn
	}
	if s.SelectionOn {
		e.request(action{selection: true})
	}
	return false
}

// convertPhrase перепечатывает весь текст, набранный после последнего перемещения курсора.
func (e *Engine) convertPhrase(switchLayout bool) {
	if !slices.ContainsFunc(e.buffer, func(st keys.Stroke) bool { return st.Key != keys.Space }) {
		return
	}
	e.request(action{word: slices.Clone(e.buffer), keepLayout: !switchLayout})
}

// tapKey возвращает клавишу из настроек, которой соответствует k, и признак клавиши фразы.
func (s *Settings) tapKey(k keys.Key) (keys.Key, bool) {
	if s.WordOn || s.SelectionOn || s.PhraseOn && s.PhraseTriple {
		if c := s.convertKey(k); c != keys.None {
			return c, false
		}
	}
	if s.PhraseOn && !s.PhraseTriple {
		for _, p := range s.Phrase {
			if p.Matches(k) {
				return p, true
			}
		}
	}
	return keys.None, false
}
