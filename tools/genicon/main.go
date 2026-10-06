// Команда genicon рисует иконку приложения (клавиша с буквами A и Я) в PNG.
// С флагом -tray — упрощённую иконку для трея, различимую в 16 px.
//
//	go run ./tools/genicon -o assets/icon.png
//	go run ./tools/genicon -tray -o assets/tray.png
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

func main() {
	out := flag.String("o", "assets/icon.png", "куда сохранить PNG")
	size := flag.Int("size", 256, "размер в пикселях")
	tray := flag.Bool("tray", false, "упрощённая иконка для трея (одна буква A)")
	flag.Parse()

	draw := render
	if *tray {
		draw = renderTray
	}
	img, err := draw(*size)
	if err != nil {
		log.Fatal(err)
	}
	if err := save(*out, img); err != nil {
		log.Fatal(err)
	}
}

// render рисует иконку в координатах 64×64, масштабированных до size.
func render(size int) (*image.RGBA, error) {
	s := float32(size) / 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	roundRect(img, 4*s, 4*s, 60*s, 60*s, 12*s, color.RGBA{0x2B, 0x5F, 0xD9, 0xFF}) // нижняя грань клавиши
	roundRect(img, 4*s, 4*s, 60*s, 54*s, 12*s, color.RGBA{0x3B, 0x7B, 0xF5, 0xFF}) // верхняя грань

	f, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, err
	}
	if err := text(img, f, "A", 26*s, 20*s, 34*s, color.White); err != nil {
		return nil, err
	}
	if err := text(img, f, "Я", 20*s, 34*s, 50*s, color.RGBA{0xCF, 0xE0, 0xFF, 0xFF}); err != nil {
		return nil, err
	}
	return img, nil
}

// roundRect закрашивает прямоугольник со скруглёнными углами (со сглаживанием).
func roundRect(dst *image.RGBA, x0, y0, x1, y1, r float32, c color.Color) {
	const k = 0.5523 // кубическая кривая, приближающая четверть окружности
	z := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
	z.MoveTo(x0+r, y0)
	z.LineTo(x1-r, y0)
	z.CubeTo(x1-r+k*r, y0, x1, y0+r-k*r, x1, y0+r)
	z.LineTo(x1, y1-r)
	z.CubeTo(x1, y1-r+k*r, x1-r+k*r, y1, x1-r, y1)
	z.LineTo(x0+r, y1)
	z.CubeTo(x0+r-k*r, y1, x0, y1-r+k*r, x0, y1-r)
	z.LineTo(x0, y0+r)
	z.CubeTo(x0, y0+r-k*r, x0+r-k*r, y0, x0+r, y0)
	z.ClosePath()
	z.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{})
}

// text рисует строку s кеглем size; (x, y) — начало базовой линии.
func text(dst *image.RGBA, f *opentype.Font, s string, size, x, y float32, c color.Color) error {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return err
	}
	defer face.Close()
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)},
	}
	d.DrawString(s)
	return nil
}

func save(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// renderTray рисует иконку для трея: крупная буква A во весь квадрат,
// без мелких деталей, которые теряются при уменьшении до 16 px.
func renderTray(size int) (*image.RGBA, error) {
	s := float32(size) / 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	roundRect(img, 0, 0, 64*s, 64*s, 12*s, color.RGBA{0x2B, 0x6C, 0xF0, 0xFF})

	f, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, err
	}
	if err := centeredText(img, f, "A", 52*s, color.White); err != nil {
		return nil, err
	}
	return img, nil
}

// centeredText рисует строку s кеглем size по центру изображения (по контуру глифов).
func centeredText(dst *image.RGBA, f *opentype.Font, s string, size float32, c color.Color) error {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return err
	}
	defer face.Close()
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: face}
	b, _ := d.BoundString(s) // границы относительно Dot = (0, 0)
	w, h := fixed.I(dst.Bounds().Dx()), fixed.I(dst.Bounds().Dy())
	d.Dot = fixed.Point26_6{
		X: (w-(b.Max.X-b.Min.X))/2 - b.Min.X,
		Y: (h-(b.Max.Y-b.Min.Y))/2 - b.Min.Y,
	}
	d.DrawString(s)
	return nil
}
