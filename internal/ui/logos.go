package ui

import (
	"fmt"
	"slices"
	"strings"

	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

const logosPerPage = 2

func (d *desktop) chooseLogos() {
	d.pick(mygo.OpenDialogOptions{
		Title: d.t("watermark.choosePNG"), ButtonLabel: d.t("common.use"), Multiple: true,
		Filters: []mygo.FileFilter{{Name: "PNG", Extensions: []string{"png"}}},
	}, d.model.AddLogos)
}

func (d *desktop) syncLogoSelection() {
	if revision := d.model.LogoImportRevision; revision != d.seenLogoImport {
		d.seenLogoImport = revision
		d.selectLogo(d.model.LastAddedLogo, d.model.Image.LogoMarks())
	}
	if d.logoSelection != "" && d.model.Image.LogoByPath(d.logoSelection) == nil {
		d.logoSelection = ""
	}
}

func (d *desktop) selectLogo(path string, marks []imaging.LogoMark) {
	if i := slices.IndexFunc(marks, func(l imaging.LogoMark) bool { return l.Path == path }); i >= 0 {
		d.logoSelection, d.logoPage = path, i/logosPerPage
	}
}

func (d *desktop) logoControls(c *ui.Context, before, o *imaging.Options, h *historyControls) {
	tk := theme.TokensOf(c)
	marks := o.LogoMarks()
	start, end := pageRange(len(marks), logosPerPage, &d.logoPage)
	strip := ui.Row(c.Key("logo-strip")).AlignSelf(ui.Stretch).Gap(7)
	if strip.FileDragOver() {
		strip.Radius(tk.BorderRadiusSM).Background(tk.FillTertiary)
	}
	if paths := strip.DroppedFiles(); len(paths) != 0 {
		d.model.AddLogos(paths)
	}
	strip.Children(func() {
		for _, mark := range marks[start:end] {
			d.logoTile(c.Key(mark.Path), mark.Path, mark.Path == d.logoSelection)
		}
		full := len(marks) == imaging.MaxLogoLayers
		add := ui.ButtonBase(c.Key("add")).Column().Grow(1).Basis(0).MinWidth(0).Height(62).Shrink(0).
			Center().Gap(5).Radius(tk.BorderRadiusSM).Border(1, tk.Border).BorderStyle(ui.BorderDashed).
			Label(tr(c, "watermark.addLogos")).Tooltip(tr(c, "watermark.choosePNG")).Cursor(ui.CursorPointer).
			Disabled(d.model.LogoPending || full)
		if add.Hovered() && !full {
			add.Background(tk.FillQuaternary)
		}
		add.Children(func() {
			icon(c, lucide.Plus(), 17, tk.TextSecondary)
			strong(c, tr(c, "watermark.addLogos")).FontSize(11).MaxWidthPercent(100).SingleLine()
		})
		if add.Clicked() {
			d.chooseLogos()
		}
	})
	ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
		switch {
		case d.model.LogoPending:
			note(c, tr(c, "watermark.validatingPNG"))
		case len(marks) == imaging.MaxLogoLayers:
			note(c, trf(c, "watermark.maxLogos", l10n.Args{"count": imaging.MaxLogoLayers}))
		default:
			ui.Text(c, tr(c, "watermark.multiPNGHint")).Wrap().FontSize(11).TextColor(tk.TextSecondary).Grow(1)
		}
		if len(marks) > logosPerPage {
			ui.Box(c).Grow(1)
			arrows(c.Key("logo-pages"), len(marks), logosPerPage, &d.logoPage)
		}
	})
	d.logoSettings(c.Key(d.logoSelection), before, o, marks, h)
}

func (d *desktop) logoSettings(c *ui.Context, before, o *imaging.Options, marks []imaging.LogoMark, h *historyControls) {
	tk := theme.TokensOf(c)
	l := o.LogoByPath(d.logoSelection)
	if l == nil {
		if len(marks) != 0 {
			line(c)
			note(c, tr(c, "watermark.selectLogoHint"))
			return
		}
		l = &o.Logo // Editable defaults for the next import.
	} else {
		line(c)
		remove := false
		ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
			icon(c, lucide.FileImage(), 14, tk.TextSecondary)
			strong(c, baseName(c, l.Path)).FontSize(12).Grow(1).MinWidth(0).Shrink(1).Tooltip(l.Path)
			remove = smallIcon(c, lucide.X(), tr(c, "watermark.removeLogo"), false, false)
		})
		if remove {
			d.removeLogo(o, marks, l.Path)
			l = o.LogoByPath(d.logoSelection)
			if l == nil {
				l = &o.Logo
			}
		}
	}
	old := before.LogoByPath(l.Path)
	if old == nil {
		old = &before.Logo
	}
	h.slider(c, "watermark.widthPercent", &old.Width, &l.Width, 1, 100, false)
	if !l.KeepRatio {
		h.slider(c, "watermark.heightPercent", &old.Height, &l.Height, 1, 100, false)
	}
	h.slider(c, "watermark.opacity", &old.Opacity, &l.Opacity, 0, 100, false)
	choice.Checkbox(c, &l.KeepRatio, tr(c, "watermark.keepRatio"), choice.CheckboxProps{})
	line(c)
	placement(c, &l.Placement)
}

func (d *desktop) logoTile(c *ui.Context, path string, selected bool) {
	tk := theme.TokensOf(c)
	name := baseName(c, path)
	tile := ui.ButtonBase(c).Column().Grow(1).Basis(0).MinWidth(0).Height(62).Shrink(0).
		Center().Gap(5).Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).Label(name).Tooltip(path).
		Role(ui.RoleToggleButton).Checked(selected)
	bg, border := tk.FillQuaternary, tk.BorderSecondary
	switch {
	case selected:
		bg, border = tk.FillSecondary, tk.TextTertiary
	case tile.Hovered():
		bg, border = tk.FillTertiary, tk.Border
	}
	tile.Background(bg).Border(1, border).Children(func() {
		sw := ui.Box(c).Size(54, 25).Radius(4).Center().Clip().Draw(func(p *ui.Painter, r ui.Rect) {
			const cell = 5
			for row, y := 0, float32(0); y < r.H; row, y = row+1, y+cell {
				for col, x := 0, float32(0); x < r.W; col, x = col+1, x+cell {
					p.Fill(ui.Rect{X: r.X + x, Y: r.Y + y, W: min(cell, r.W-x), H: min(cell, r.H-y)},
						choose((row+col)%2 == 0, tk.BgElevated, tk.FillSecondary), 0)
				}
			}
		})
		sw.Children(func() {
			if pixels := d.model.LogoThumbnail(path); pixels != nil {
				display.Image(c, bitmap(tile, pixels), display.ImageProps{
					Width: 47, Height: 20, Contain: true, NoPreview: true, Variant: display.ImageBorderless,
				}).Radius(0).IgnoreInput(true)
			} else {
				icon(c, lucide.Image(), 14, tk.TextTertiary)
			}
		})
		ui.Text(c, strings.TrimSuffix(name, ".png")).SingleLine().FontSize(11).MaxWidthPercent(100).
			FontWeight(choose(selected, 600, 450)).TextColor(choose(selected, tk.Text, tk.TextSecondary))
	})
	if tile.Clicked() && !selected {
		d.selectLogo(path, d.model.Image.LogoMarks())
		c.Invalidate()
	}
}

func (d *desktop) removeLogo(o *imaging.Options, marks []imaging.LogoMark, path string) {
	i := slices.IndexFunc(marks, func(l imaging.LogoMark) bool { return l.Path == path })
	if i < 0 {
		return
	}
	removed := marks[i]
	pos := o.TextLayerPosition(len(marks))
	if i < pos {
		pos--
	}
	marks = slices.Delete(marks, i, i+1)
	o.SetLogoMarks(marks)
	o.SetTextLayerPosition(pos, len(marks))
	if len(marks) == 0 {
		removed.Path, removed.Enabled = "", false
		o.Logo, o.LogoOnTop = removed, true
		d.logoSelection, d.logoPage = "", 0
	} else {
		d.selectLogo(marks[min(i, len(marks)-1)].Path, marks)
	}
}

func (d *desktop) layersControls(c *ui.Context, o *imaging.Options) {
	type item struct {
		text bool
		logo imaging.LogoMark
	}
	marks := o.LogoMarks()
	items := make([]item, 0, len(marks)+1)
	pos := o.TextLayerPosition(len(marks))
	for i := len(marks); i >= 0; i-- {
		if i == pos && o.Text.Enabled {
			items = append(items, item{text: true})
		}
		if i > 0 {
			items = append(items, item{logo: marks[i-1]})
		}
	}
	if len(items) == 0 {
		note(c, tr(c, "watermark.emptyLayers"))
		return
	}
	if len(items) > 1 {
		strong(c, tr(c, "watermark.layerOrder"))
	}
	tk := theme.TokensOf(c)
	row := func(l item, handle func()) {
		name, svg, p := tr(c, "watermark.text"), lucide.Type(), o.Text.Placement
		if !l.text {
			name, svg, p = baseName(c, l.logo.Path), lucide.Image(), l.logo.Placement
		}
		ui.Row(c).Height(54).Grow(1).PaddingX(6).Gap(6).AlignItems(ui.Center).Children(func() {
			if handle != nil {
				handle()
			}
			swatch := ui.Box(c).Size(28, 28).Shrink(0).Center().Radius(tk.BorderRadiusSM).Background(tk.FillQuaternary)
			swatch.Children(func() {
				if pixels := d.model.LogoThumbnail(l.logo.Path); !l.text && pixels != nil {
					display.Image(c, bitmap(swatch, pixels), display.ImageProps{
						Size: 22, Contain: true, NoPreview: true, Variant: display.ImageBorderless,
					})
				} else {
					icon(c, svg, 16, tk.TextSecondary)
				}
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
				strong(c, name).FontSize(12).Tooltip(name)
				muted(c, fmt.Sprintf("%s · %d%%", placementLabel(c, p), p.Opacity))
			})
			if smallIcon(c, lucide.ChevronRight(), trf(c, "watermark.editLayer", l10n.Args{"layer": name}), false, false) {
				d.watermarkTab = 0
				if !l.text {
					d.watermarkTab = 1
					d.selectLogo(l.logo.Path, marks)
				}
				c.Invalidate()
			}
		})
	}
	ui.Column(c).Radius(tk.BorderRadiusLG).Border(1, tk.BorderSecondary).Children(func() {
		if len(items) == 1 {
			row(items[0], nil)
		} else {
			layout.SortableList(c, &items, func(l item) string { return l.logo.Path }, layout.SortableListProps{Gap: 1}, row)
		}
	})
	if len(items) > 1 {
		sorted := marks[:0]
		for i := len(items) - 1; i >= 0; i-- {
			if items[i].text {
				pos = len(sorted)
			} else {
				sorted = append(sorted, items[i].logo)
			}
		}
		o.SetLogoMarks(sorted)
		if o.Text.Enabled {
			o.SetTextLayerPosition(pos, len(sorted))
		}
		note(c, tr(c, "watermark.reorderHint"))
	}
}
