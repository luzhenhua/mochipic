package ui

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/mochipic-128.png
var brandIconPNG []byte

//go:embed assets/mark-github-16.svg
var githubMarkSVG []byte

var githubMark = ui.MustParseSVG(githubMarkSVG)

func brandIcon(c *ui.Context) {
	box := ui.Box(c).Size(35, 35).Shrink(0)
	bitmap := ui.Local(box, "brand-icon", func() *ui.Bitmap {
		b, _ := ui.DecodeBitmap(brandIconPNG)
		return b
	})
	if *bitmap != nil {
		box.Children(func() { ui.Image(c, *bitmap).Fill().Fit(ui.Contain) })
	}
}
