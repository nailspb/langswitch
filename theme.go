package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Палитра тёмного интерфейса в духе настроек macOS.
var (
	colBg            = rgb(0x0e0e10) // фон окна
	colBgElev        = rgb(0x161618) // боковая панель, карточки
	colBgElev2       = rgb(0x1c1c1f) // выбранный пункт меню
	colBgHover       = rgb(0x202024) // наведение, плашки клавиш
	colBgInput       = rgb(0x141417)
	colBorder        = color.NRGBA{R: 255, G: 255, B: 255, A: 20} // 8 %
	colBorderStrong  = color.NRGBA{R: 255, G: 255, B: 255, A: 36} // 14 %
	colText          = rgb(0xecedee)
	colTextSecondary = rgb(0xa0a0a8)
	colTextTertiary  = rgb(0x6e6e76)
	colAccent        = rgb(0x3b82f6)
	colRed           = rgb(0xef4444)
	colGreen         = rgb(0x30d158)
	colOrange        = rgb(0xf59e0b)
	colSwitchOff     = rgb(0x3a3a40)
)

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
}

// appTheme — тёмная тема приложения; всё, что не задано, берётся из тёмной темы Fyne.
type appTheme struct{}

var themeColors = map[fyne.ThemeColorName]color.Color{
	theme.ColorNameBackground:          colBg,
	theme.ColorNameButton:              rgb(0x232327),
	theme.ColorNameDisabledButton:      colBgElev2,
	theme.ColorNameDisabled:            colTextTertiary,
	theme.ColorNameError:               colRed,
	theme.ColorNameFocus:               colAccent,
	theme.ColorNameForeground:          colText,
	theme.ColorNameForegroundOnPrimary: color.White,
	theme.ColorNameHover:               color.NRGBA{R: 255, G: 255, B: 255, A: 14},
	theme.ColorNameHeaderBackground:    colBgElev,
	theme.ColorNameHyperlink:           rgb(0x6ea8ff),
	theme.ColorNameInputBackground:     colBgInput,
	theme.ColorNameInputBorder:         colBorderStrong,
	theme.ColorNameMenuBackground:      rgb(0x242428),
	theme.ColorNameOverlayBackground:   colBgElev2,
	theme.ColorNamePlaceHolder:         colTextTertiary,
	theme.ColorNamePressed:             color.NRGBA{R: 255, G: 255, B: 255, A: 26},
	theme.ColorNamePrimary:             colAccent,
	theme.ColorNameScrollBar:           color.NRGBA{R: 255, G: 255, B: 255, A: 40},
	theme.ColorNameSelection:           color.NRGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0x55},
	theme.ColorNameSeparator:           colBorder,
	theme.ColorNameShadow:              color.NRGBA{A: 0x66},
	theme.ColorNameSuccess:             colGreen,
	theme.ColorNameWarning:             colOrange,
}

var themeSizes = map[fyne.ThemeSizeName]float32{
	theme.SizeNameText:            14,
	theme.SizeNameCaptionText:     12,
	theme.SizeNameSubHeadingText:  16,
	theme.SizeNameHeadingText:     26,
	theme.SizeNameInnerPadding:    8,
	theme.SizeNameInputRadius:     8,
	theme.SizeNameButtonRadius:    8,
	theme.SizeNameSelectionRadius: 6,
}

func (appTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if c, ok := themeColors[n]; ok {
		return c
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (appTheme) Font(s fyne.TextStyle) fyne.Resource { return theme.DefaultTheme().Font(s) }

func (appTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }

func (appTheme) Size(n fyne.ThemeSizeName) float32 {
	if s, ok := themeSizes[n]; ok {
		return s
	}
	return theme.DefaultTheme().Size(n)
}
