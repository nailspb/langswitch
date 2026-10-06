package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Элементы интерфейса в стиле настроек macOS: карточки, строки настроек, переключатели.

const (
	toggleW, toggleH = 38, 22
	thumbSize        = 18
	navIconSize      = 18
)

// toggle — переключатель-«свитч».
type toggle struct {
	widget.BaseWidget
	On        bool
	OnChanged func(on bool)
}

func newToggle(on bool, changed func(on bool)) *toggle {
	t := &toggle{On: on, OnChanged: changed}
	t.ExtendBaseWidget(t)
	return t
}

func (t *toggle) Tapped(*fyne.PointEvent) {
	t.On = !t.On
	t.Refresh()
	if t.OnChanged != nil {
		t.OnChanged(t.On)
	}
}

func (t *toggle) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (t *toggle) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(colSwitchOff)
	track.CornerRadius = toggleH / 2
	r := &toggleRenderer{t: t, track: track, thumb: canvas.NewCircle(color.White)}
	r.Refresh()
	return r
}

type toggleRenderer struct {
	t     *toggle
	track *canvas.Rectangle
	thumb *canvas.Circle
}

func (r *toggleRenderer) Layout(size fyne.Size) {
	y := (size.Height - toggleH) / 2
	r.track.Move(fyne.NewPos(0, y))
	r.track.Resize(fyne.NewSize(toggleW, toggleH))
	x := float32(2)
	if r.t.On {
		x = toggleW - thumbSize - 2
	}
	r.thumb.Move(fyne.NewPos(x, y+2))
	r.thumb.Resize(fyne.NewSquareSize(thumbSize))
}

func (r *toggleRenderer) MinSize() fyne.Size { return fyne.NewSize(toggleW, toggleH) }

func (r *toggleRenderer) Refresh() {
	r.track.FillColor = colSwitchOff
	if r.t.On {
		r.track.FillColor = colAccent
	}
	r.Layout(r.t.Size())
	r.track.Refresh()
	r.thumb.Refresh()
}

func (r *toggleRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.track, r.thumb} }

func (r *toggleRenderer) Destroy() {}

// pill — выбираемый элемент: пункт бокового меню (со значком) или сегмент переключателя режимов.
type pill struct {
	widget.BaseWidget
	text     string
	textSize float32
	radius   float32
	pad      fyne.Size // отступы текста по горизонтали и вертикали
	ring     bool      // обводка у выбранного (для сегментов)
	iconOn   fyne.Resource
	iconOff  fyne.Resource
	selected bool
	hovered  bool
	onTap    func()
}

func newNavItem(icon fyne.Resource, text string, onTap func()) *pill {
	p := &pill{text: text, textSize: 15, radius: 8, pad: fyne.NewSize(12, 10), onTap: onTap,
		iconOn: theme.NewPrimaryThemedResource(icon), iconOff: theme.NewDisabledResource(icon)}
	p.ExtendBaseWidget(p)
	return p
}

func newSegment(text string, onTap func()) *pill {
	p := &pill{text: text, textSize: 13, radius: 7, pad: fyne.NewSize(12, 5), ring: true, onTap: onTap}
	p.ExtendBaseWidget(p)
	return p
}

func (p *pill) setSelected(on bool) {
	p.selected = on
	p.Refresh()
}

func (p *pill) Tapped(*fyne.PointEvent) {
	if p.onTap != nil {
		p.onTap()
	}
}

func (p *pill) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (p *pill) MouseIn(*desktop.MouseEvent) {
	p.hovered = true
	p.Refresh()
}

func (p *pill) MouseMoved(*desktop.MouseEvent) {}

func (p *pill) MouseOut() {
	p.hovered = false
	p.Refresh()
}

func (p *pill) CreateRenderer() fyne.WidgetRenderer {
	text := canvas.NewText(p.text, colTextSecondary)
	text.TextSize = p.textSize
	r := &pillRenderer{p: p, bg: canvas.NewRectangle(color.Transparent), text: text}
	r.bg.CornerRadius = p.radius
	if p.iconOff != nil {
		r.icon = canvas.NewImageFromResource(p.iconOff)
		r.icon.FillMode = canvas.ImageFillContain
	}
	r.Refresh()
	return r
}

type pillRenderer struct {
	p    *pill
	bg   *canvas.Rectangle
	icon *canvas.Image // nil у сегментов
	text *canvas.Text
}

func (r *pillRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	x := r.p.pad.Width
	if r.icon != nil {
		r.icon.Resize(fyne.NewSquareSize(navIconSize))
		r.icon.Move(fyne.NewPos(x, (size.Height-navIconSize)/2))
		x += navIconSize + 12
	}
	ts := r.text.MinSize()
	r.text.Move(fyne.NewPos(x, (size.Height-ts.Height)/2))
}

func (r *pillRenderer) MinSize() fyne.Size {
	ts := r.text.MinSize()
	w, h := ts.Width+2*r.p.pad.Width, ts.Height
	if r.icon != nil {
		w += navIconSize + 12
		h = max(h, navIconSize)
	}
	return fyne.NewSize(w, h+2*r.p.pad.Height)
}

func (r *pillRenderer) Refresh() {
	p := r.p
	r.bg.FillColor, r.bg.StrokeWidth = color.Transparent, 0
	switch {
	case p.selected:
		r.bg.FillColor = colBgElev2
		if p.ring {
			r.bg.StrokeColor, r.bg.StrokeWidth = colBorderStrong, 1
		}
	case p.hovered:
		r.bg.FillColor = colBgHover
	}
	r.text.Color = colTextSecondary
	if p.selected || p.hovered {
		r.text.Color = colText
	}
	if r.icon != nil {
		r.icon.Resource = p.iconOff
		if p.selected {
			r.icon.Resource = p.iconOn
		}
		r.icon.Refresh()
	}
	r.bg.Refresh()
	r.text.Refresh()
}

func (r *pillRenderer) Objects() []fyne.CanvasObject {
	if r.icon == nil {
		return []fyne.CanvasObject{r.bg, r.text}
	}
	return []fyne.CanvasObject{r.bg, r.icon, r.text}
}

func (r *pillRenderer) Destroy() {}

// segmented — переключатель режимов из нескольких сегментов.
func segmented(options []string, selected int, changed func(i int)) fyne.CanvasObject {
	items := make([]*pill, len(options))
	objs := make([]fyne.CanvasObject, len(options))
	for i, o := range options {
		items[i] = newSegment(o, func() {
			if items[i].selected {
				return
			}
			for j, it := range items {
				it.setSelected(j == i)
			}
			changed(i)
		})
		items[i].selected = i == selected
		objs[i] = items[i]
	}
	bg := canvas.NewRectangle(colBgInput)
	bg.StrokeColor, bg.StrokeWidth, bg.CornerRadius = colBorder, 1, 9
	return container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(3, 3, 3, 3), container.NewHBox(objs...)))
}

// card — карточка: тёмная подложка со скруглением и тонкой рамкой.
func card(objs ...fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colBgElev)
	bg.StrokeColor, bg.StrokeWidth, bg.CornerRadius = colBorder, 1, 12
	return container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(6, 6, 12, 12), container.NewVBox(objs...)))
}

// row — строка настройки: название и пояснение слева, элемент управления справа.
func row(title, hint string, control fyne.CanvasObject) fyne.CanvasObject {
	return rowWith(title, hintLabel(hint), control)
}

func rowWith(title string, hint *widget.Label, control fyne.CanvasObject) fyne.CanvasObject {
	text := fyne.CanvasObject(widget.NewLabel(title))
	if hint != nil {
		// Подписи у Label с внутренними отступами — сближаем название и пояснение.
		text = container.New(layout.NewCustomPaddedVBoxLayout(-12), text, hint)
	}
	if control == nil {
		return text
	}
	return container.NewBorder(nil, nil, nil, vcenter(control), text)
}

func hintLabel(s string) *widget.Label {
	if s == "" {
		return nil
	}
	l := widget.NewLabel(s)
	l.Importance = widget.LowImportance
	l.SizeName = theme.SizeNameCaptionText
	l.Wrapping = fyne.TextWrapWord
	return l
}

func vcenter(o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(layout.NewSpacer(), o, layout.NewSpacer())
}

// caption — подпись над карточкой, как заголовок колонки таблицы.
func caption(s string) fyne.CanvasObject {
	l := widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.Importance = widget.LowImportance
	l.SizeName = theme.SizeNameCaptionText
	return container.New(layout.NewCustomPaddedLayout(10, -6, 0, 0), l)
}

func separator() fyne.CanvasObject {
	r := canvas.NewRectangle(colBorder)
	r.SetMinSize(fyne.NewSize(0, 1))
	return r
}

// chip — плашка с названием клавиши.
func chip(s string) fyne.CanvasObject {
	bg := canvas.NewRectangle(colBgHover)
	bg.CornerRadius = 6
	t := canvas.NewText(s, colText)
	t.TextSize = 13
	t.TextStyle.Bold = true
	return container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(4, 4, 10, 10), t))
}

// fixedWidth задаёт объекту минимальную ширину.
func fixedWidth(w float32, o fyne.CanvasObject) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(w, 0))
	return container.NewStack(r, o)
}

// page — страница настроек: заголовок, подзаголовок и секции.
func page(title, subtitle string, sections ...fyne.CanvasObject) fyne.CanvasObject {
	t := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	t.SizeName = theme.SizeNameHeadingText
	body := container.NewVBox(t)
	if sub := hintLabel(subtitle); sub != nil {
		sub.SizeName = theme.SizeNameText
		body.Add(container.New(layout.NewCustomPaddedLayout(-10, 6, 0, 0), sub))
	}
	for _, s := range sections {
		body.Add(s)
	}
	return container.NewVScroll(container.New(layout.NewCustomPaddedLayout(18, 28, 24, 24), body))
}

// callout — плашка с ошибкой.
type callout struct {
	box   *fyne.Container
	label *widget.Label
}

func newCallout() *callout {
	bg := canvas.NewRectangle(color.NRGBA{R: 0xef, G: 0x44, B: 0x44, A: 26})
	bg.StrokeColor, bg.StrokeWidth, bg.CornerRadius = color.NRGBA{R: 0xef, G: 0x44, B: 0x44, A: 64}, 1, 10
	l := widget.NewLabel("")
	l.Importance = widget.DangerImportance
	l.Wrapping = fyne.TextWrapWord
	c := &callout{box: container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(2, 2, 6, 6), l)), label: l}
	c.box.Hide()
	return c
}

func (c *callout) set(msg string) {
	c.label.SetText(msg)
	if msg == "" {
		c.box.Hide()
	} else {
		c.box.Show()
	}
}
