// Package caption holds per-photo title strips and reusable appearance settings.
package caption

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxRunes     = 240
	MinSize      = 1
	MaxSize      = 12
	MinHeight    = 8
	MaxHeight    = 60
	ManualHeight = 18
)

type Alignment int

const (
	Center Alignment = iota
	Left
	Right
)

type Style struct {
	Background, TextColor string
	Opacity               int
	Size                  int // Percent of the final image width.
	Height                int // Zero fits to the title; otherwise percent of image height.
	Alignment             Alignment
}

func Defaults() Style {
	return Style{Background: "#000000", TextColor: "#FFFFFF", Opacity: 40, Size: 4}
}

func (s Style) Valid() bool {
	return hexColor(s.Background) && hexColor(s.TextColor) && s.Opacity >= 0 && s.Opacity <= 100 &&
		s.Size >= MinSize && s.Size <= MaxSize && (s.Height == 0 || s.Height >= MinHeight && s.Height <= MaxHeight) &&
		s.Alignment >= Center && s.Alignment <= Right
}

func hexColor(v string) bool {
	if len(v) != 7 || v[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(v[1:], 16, 24)
	return err == nil
}

type Caption struct {
	Text  string
	Style Style
}

// Typography deliberately excludes the background. Its color and opacity can
// be changed without laying out or rasterizing the title again.
type Typography struct {
	Text, Color  string
	Size, Height int
	Alignment    Alignment
}

func (c Caption) Typography() Typography {
	return Typography{strings.TrimSpace(c.Text), c.Style.TextColor, c.Style.Size, c.Style.Height, c.Style.Alignment}
}

func (c Caption) Active() bool {
	return strings.ContainsFunc(c.Text, func(r rune) bool { return unicode.IsGraphic(r) && !unicode.IsSpace(r) })
}

func (c Caption) Valid() bool {
	return c.Style.Valid() && len(c.Text) <= MaxRunes*utf8.UTFMax && c.Text == CleanText(c.Text)
}

func CleanText(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if utf8.RuneCountInString(s) <= MaxRunes && utf8.ValidString(s) && !strings.ContainsFunc(s, func(r rune) bool {
		return r != '\n' && unicode.IsControl(r)
	}) {
		return s
	}
	var out strings.Builder
	count := 0
	for _, r := range s {
		if r == '\t' {
			r = ' '
		}
		if r != '\n' && unicode.IsControl(r) {
			continue
		}
		out.WriteRune(r)
		count++
		if count == MaxRunes {
			break
		}
	}
	return out.String()
}
