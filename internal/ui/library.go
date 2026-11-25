package ui

import (
	"cmp"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func pageRange(total, per int, p *int) (int, int) {
	*p = max(0, min(*p, max(0, (total-1)/per)))
	start := *p * per
	return start, min(total, start+per)
}
func rangeText(start, end, total int) string {
	if total == 0 {
		return "0 / 0"
	}
	return fmt.Sprintf("%d–%d / %d", start+1, end, total)
}
func arrows(c *ui.Context, total, per int, p *int) {
	if smallIcon(c, lucide.ChevronLeft(), tr(c, "pager.previous"), false, *p <= 0) {
		*p--
	}
	if smallIcon(c, lucide.ChevronRight(), tr(c, "pager.next"), false, (*p+1)*per >= total) {
		*p++
	}
}

func (d *desktop) libraryItems() []files.Source {
	return d.sortedLibrary(d.model.Sources, d.listQuery)
}

func (d *desktop) sortedLibrary(sources []files.Source, search string) []files.Source {
	if search == "" && d.listSort == "order" {
		return sources
	}
	query := strings.ToLower(strings.TrimSpace(search))
	items := make([]files.Source, 0, len(sources))
	for _, s := range sources {
		if strings.Contains(strings.ToLower(filepath.Base(s.Path)), query) {
			items = append(items, s)
		}
	}
	switch d.listSort {
	case "name":
		slices.SortStableFunc(items, func(a, b files.Source) int {
			return cmp.Compare(strings.ToLower(filepath.Base(a.Path)), strings.ToLower(filepath.Base(b.Path)))
		})
	case "size":
		slices.SortStableFunc(items, func(a, b files.Source) int { return cmp.Compare(b.Size, a.Size) })
	}
	return items
}

func pager(c *ui.Context, total, per int, p *int) {
	start, end := pageRange(total, per, p)
	muted(c, rangeText(start, end, total))
	arrows(c, total, per, p)
}

const listPerPage = 12

func (d *desktop) library(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	if len(m.Sources) == 0 {
		d.listVisible = 0
		return
	}
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).WidthPercent(100).MaxWidth(1260).AlignSelf(ui.Center).Padding(18, 24, 0, 24).Gap(12).Children(func() {
		ui.Row(c).Height(38).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
			old, sort := d.listQuery, d.listSort
			ui.Column(c).Width(min(224, max(140, d.centerWidth/4))).Shrink(0).Children(func() {
				field.Input(c.Key("library-search"), &d.listQuery, field.InputProps{Placeholder: tr(c, "library.searchNames"), Prefix: lucide.Search(), Variant: field.InputFilled}).Label(tr(c, "library.searchNames"))
			})
			ui.Box(c).Grow(1).MinWidth(0)
			if styledButton(c.Key("open-collage"), tr(c, "nav.collage"), lucide.Grid2X2(), action.Default, len(d.model.EditSelection) < 2 && !m.HasCollage() || m.Scanning, false).
				Tooltip(tr(c, "collage.selectHint")).Clicked() {
				d.beginCollage()
			}
			if button(c, tr(c, "library.removeSelected"), lucide.CircleMinus(), action.Default, len(d.model.EditSelection) == 0 || m.Scanning) {
				d.removeSelected()
			}
			if button(c, tr(c, "library.clearList"), lucide.Trash2(), action.Text, m.Scanning) {
				d.clearQueue()
			}
			field.Select(c.Key("library-sort"), &d.listSort, field.SelectProps{Options: []field.SelectOption{
				{Value: "order", Label: tr(c, "library.importOrder")}, {Value: "name", Label: tr(c, "common.filename")}, {Value: "size", Label: tr(c, "library.sortSize")},
			}}).Width(104).Label(tr(c, "library.sort"))
			if old != d.listQuery || sort != d.listSort {
				d.listPage = 0
			}
		})
		items := d.libraryItems()
		d.listVisible = len(items)
		start, end := pageRange(len(items), listPerPage, &d.listPage)
		m.ShowThumbnails(pathsOf(items[start:end]))
		ui.Column(c).Grow(1).Basis(0).MinHeight(0).Children(func() {
			ui.Row(c).Shrink(0).Height(38).PaddingX(12).AlignItems(ui.Center).Gap(12).Radius(tk.BorderRadiusSM).Background(tk.BgLayout).Children(func() {
				n := 0
				for _, s := range items {
					if d.model.EditSelection[s.Path] {
						n++
					}
				}
				all := len(items) > 0 && n == len(items)
				v := all
				choice.Checkbox(c.Key("select-all"), &v, "", choice.CheckboxProps{Indeterminate: n > 0 && !all}).Label(tr(c, "library.selectFiltered")).Disabled(len(items) == 0)
				if v != all {
					d.selectLibraryItems(items, v)
				}
				muted(c, tr(c, "common.filename")).Grow(1).Basis(0).MinWidth(0)
				muted(c, tr(c, "common.folder")).Width(d.libraryFolderWidth())
				muted(c, tr(c, "image.dimensions")).Width(98)
				muted(c, tr(c, "nav.format")).Width(56)
				muted(c, tr(c, "library.fileSize")).Width(74)
			})
			layout.ScrollArea(c.Key(d.listPage), layout.ScrollAreaProps{}, func() {
				ui.Column(c).Children(func() {
					if len(items) == 0 {
						ui.Box(c).Height(100).Center().Children(func() { note(c, tr(c, "library.noMatches")) })
					}
					for _, s := range items[start:end] {
						d.libraryRow(c.Key(s.Path), s, items)
					}
				})
			}).Grow(1).Basis(0).MinHeight(0).AlignSelf(ui.Stretch)
		})
	})
}

func (d *desktop) libraryFolderWidth() float32 { return min(182, max(116, d.centerWidth/7)) }

func (d *desktop) libraryRow(c *ui.Context, s files.Source, items []files.Source) {
	tk := theme.TokensOf(c)
	on := d.model.EditSelection[s.Path]
	row := ui.Row(c).Height(56).PaddingX(12).Gap(12).AlignItems(ui.Center).Radius(tk.BorderRadiusSM)
	if on {
		row.Background(tk.FillQuaternary)
	}
	row.Children(func() {
		v := on
		choice.Checkbox(c, &v, "", choice.CheckboxProps{}).Label(trf(c, "common.selectFile", l10n.Args{"name": filepath.Base(s.Path)}))
		if v != on {
			d.selectLibrary(s.Path, v, c.Modifiers(), items)
		}
		open := ui.ButtonBase(c).Grow(1).Basis(0).MinWidth(0).Height(54).AlignItems(ui.Center).
			Justify(ui.Start).Gap(10).Label(baseName(c, s.Path)).Tooltip(s.Path)
		open.Children(func() {
			d.thumb(c.Key("thumb"), s.Path, 48, 37)
			strong(c, filepath.Base(s.Path)).MinWidth(0).Shrink(1)
		})
		if open.DoubleClicked() {
			d.model.Select(s.Path)
			d.list = false
			d.showSelectedThumb()
			d.model.ShowThumbnails(nil)
		} else if open.Clicked() {
			mods := open.ClickModifiers()
			if mods&(ui.Cmd|ui.Shift) == 0 {
				d.model.Select(s.Path)
				d.listAnchor = s.Path
			} else {
				d.selectLibrary(s.Path, mods&ui.Cmd == 0 || !d.model.EditSelection[s.Path], mods, items)
			}
		}
		dir := filepath.Dir(s.Path)
		muted(c, choose(s.RelativeDir == "", baseName(c, dir), s.RelativeDir)).Width(d.libraryFolderWidth()).Tooltip(dir)
		t := d.model.Thumbnail(s.Path)
		mono(c, sizeText(t.Info.Width, t.Info.Height)).Width(98)
		muted(c, formatLabel(imaging.FormatOf(s.Path))).Width(56)
		mono(c, byteText(c, s.Size)).Width(74)
	})
	line(c)
}

func (d *desktop) selectLibrary(path string, on bool, mods ui.Modifiers, items []files.Source) {
	paths := maps.Clone(d.model.EditSelection)
	if mods&ui.Shift != 0 && d.listAnchor != "" {
		a := slices.IndexFunc(items, func(s files.Source) bool { return s.Path == d.listAnchor })
		b := slices.IndexFunc(items, func(s files.Source) bool { return s.Path == path })
		if a >= 0 && b >= 0 {
			for _, s := range items[min(a, b) : max(a, b)+1] {
				paths[s.Path] = true
			}
			d.model.SetEditSelection(paths, d.model.Selected)
			return
		}
	}
	if on {
		paths[path] = true
	} else {
		delete(paths, path)
	}
	d.model.SetEditSelection(paths, d.model.Selected)
	d.listAnchor = path
}
func (d *desktop) showSelectedThumb() {
	idx := slices.IndexFunc(d.model.Sources, func(s files.Source) bool { return s.Path == d.model.Selected })
	if idx >= 0 {
		d.thumbPage = idx / d.thumbSlots()
	}
}
func (d *desktop) removeSelected() {
	d.removeQueueSources(d.model.EditSelection)
}
func (d *desktop) clearQueue() {
	paths := make(map[string]bool, len(d.model.Sources))
	for _, s := range d.model.Sources {
		paths[s.Path] = true
	}
	d.removeQueueSources(paths)
}
func (d *desktop) selectLibraryItems(items []files.Source, on bool) {
	paths := maps.Clone(d.model.EditSelection)
	for _, s := range items {
		if on {
			paths[s.Path] = true
		} else {
			delete(paths, s.Path)
		}
	}
	d.model.SetEditSelection(paths, d.model.Selected)
}
