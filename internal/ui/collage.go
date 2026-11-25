package ui

import (
	"fmt"
	"image"
	"math"
	"path/filepath"

	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type collageView struct {
	selected, stretch string
	zoom, fit         float32
	pan, strip        ui.ScrollState
}

type collageDrag struct{ path string }

func (d *desktop) beginCollage() {
	items := make([]files.Source, 0, len(d.model.EditSelection))
	for _, s := range d.sortedLibrary(d.model.Sources, "") {
		if d.model.EditSelection[s.Path] {
			items = append(items, s)
		}
	}
	if len(items) >= 2 || !d.model.HasCollage() {
		if err := d.model.BeginCollage(items); err != nil {
			d.model.Error = err
			d.redraw()
			return
		}
	}
	d.collage = collageView{}
	d.hideInspector = false
	d.navigate(collage)
}

func (d *desktop) leaveCollage() {
	d.list = true
	d.navigate(pictures)
}

func (d *desktop) exportCollage() {
	m := d.model
	if m.Collage.Exporting || d.dialog {
		return
	}
	if m.Output.Directory == "" {
		d.pick(mygo.OpenDialogOptions{Title: d.t("output.chooseOutputFolder"), ButtonLabel: d.t("common.choose"), Directory: true, CreateDirectories: true}, func(paths []string) {
			if d.page != collage {
				return
			}
			o := m.Output
			o.Directory = paths[0]
			m.SetOutput(o)
			d.exportCollage()
		})
		return
	}
	if err := m.StartCollageExport(); err != nil {
		m.Error = err
	}
	d.redraw()
}

func (d *desktop) collageToolbar(c *ui.Context) {
	s := &d.model.Collage
	if s.Exporting {
		if button(c, tr(c, choose(s.Cancelling, "task.cancelling", "common.cancel")), lucide.X(), action.Default, s.Cancelling) {
			d.model.CancelCollageExport()
		}
		button(c, tr(c, "collage.exporting"), lucide.Download(), action.Primary, true)
		return
	}
	_, err := s.Options.Geometry(len(s.Items))
	if d.hideInspector {
		d.inspectorToggle(c)
	}
	if button(c, tr(c, "collage.export"), lucide.Download(), action.Primary, s.Loading || s.Err != nil || err != nil || d.dialog) {
		d.exportCollage()
	}
}

func (d *desktop) collageViewer(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	s, v := &m.Collage, &d.collage
	g, err := s.Options.Geometry(len(s.Items))
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Padding(4, 24, 15, 24).Children(func() {
		ui.Row(c).Height(48).Shrink(0).Gap(7).AlignItems(ui.Center).Children(func() {
			icon(c, lucide.Grid2X2(), 14, tk.TextSecondary)
			strong(c, tr(c, "collage.preview")).MinWidth(0).Shrink(1)
			if s.Options.Layout == imaging.CollageCustom {
				muted(c, "·  "+tr(c, "collage.custom"))
			} else if err == nil {
				muted(c, fmt.Sprintf("·  %d × %d", g.Columns, g.Rows)).Shrink(0)
			}
			ui.Box(c).Grow(1).MinWidth(0)
			if smallIcon(c, lucide.Minus(), tr(c, "view.zoomOut"), false, err != nil) {
				d.adjustCollageZoom(.8)
			}
			percent := choose(v.zoom == -1, float32(1), v.fit*choose(v.zoom > 0, v.zoom, 1))
			zoomLabel := tr(c, "view.fit")
			if percent > 0 {
				zoomLabel = fmt.Sprintf("%.0f%%", percent*100)
			}
			muted(c, zoomLabel).MinWidth(32).Shrink(0)
			if smallIcon(c, lucide.Plus(), tr(c, "view.zoomIn"), false, err != nil) {
				d.adjustCollageZoom(1.25)
			}
			if smallIcon(c, lucide.Maximize(), tr(c, "view.fitWindow"), v.zoom == 0, err != nil) {
				d.setZoom(0)
			}
		})
		ui.Column(c).Grow(1).Basis(0).MinHeight(0).MinWidth(0).Radius(tk.BorderRadius).Background(tk.BgLayout).Clip().Children(func() {
			if err != nil {
				ui.Box(c).Grow(1).Center().Padding(18).Children(func() { note(c, errText(c, err)) })
				return
			}
			viewport := ui.ScrollBoth(c).Grow(1).Basis(0).MinHeight(0).MinWidth(0).TrackScroll(&v.pan)
			bounds := viewport.Bounds()
			vw, vh := bounds.W, bounds.H
			if vw <= 0 || vh <= 0 {
				_, wh := c.Size()
				vw, vh = d.centerWidth, max(120, wh-278)
			}
			fit := min(max(1, vw-64)/float32(g.Size.X), max(1, vh-46)/float32(g.Size.Y))
			if math.Abs(float64(v.fit-fit)) > .0001 {
				v.fit = fit
				c.Invalidate()
			}
			scale := choose(v.zoom == -1, float32(1), fit*choose(v.zoom > 0, v.zoom, 1))
			w, h := float32(g.Size.X)*scale, float32(g.Size.Y)*scale
			if dx, dy, held := viewport.Dragged(); held && s.Options.Layout != imaging.CollageCustom && (dx != 0 || dy != 0) {
				v.pan.X, v.pan.Y = clampScroll(v.pan.X-dx, v.pan.MaxX), clampScroll(v.pan.Y-dy, v.pan.MaxY)
			}
			viewport.Children(func() {
				ui.Box(c).Size(max(vw, w+32), max(vh, h+32)).Center().Children(func() { d.drawCollage(c, g, w, h) })
			})
		})
		ui.Row(c).Height(33).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
			issue := s.Err
			if issue == nil {
				issue = s.LayerErr
			}
			if issue != nil {
				icon(c, lucide.CircleAlert(), 14, tk.Error)
				muted(c, errText(c, issue)).Grow(1).MinWidth(0).Shrink(1).Tooltip(errText(c, issue))
			} else if s.Loading {
				muted(c, tr(c, "collage.preparing")).Grow(1)
			} else {
				icon(c, lucide.GripVertical(), 13, tk.TextSecondary)
				muted(c, tr(c, "collage.reorderHint")).Grow(1).MinWidth(0).Shrink(1)
			}
			if err == nil {
				muted(c, sizeText(g.Size.X, g.Size.Y)+" · "+formatLabel(m.Image.Format)).Shrink(0)
			}
		})
		d.collageStrip(c.Key("collage-strip"))
	})
}

func clampScroll(v, upper float32) float32 { return max(0, min(upper, v)) }

func (d *desktop) adjustCollageZoom(factor float32) {
	v := &d.collage
	z := v.zoom
	if z == -1 {
		z = 1 / max(.0001, v.fit)
	} else if z == 0 {
		z = 1
	}
	d.setZoom(max(.25, min(8, z*factor)))
}

func (d *desktop) drawCollage(c *ui.Context, g imaging.CollageGeometry, w, h float32) {
	tk, s := theme.TokensOf(c), &d.model.Collage
	r, green, b, _ := imaging.ParseColor(s.Options.Background)
	scaleX, scaleY := w/float32(g.Size.X), h/float32(g.Size.Y)
	canvas := ui.Box(c).Size(w, h).Radius(3).Clip().Background(ui.RGB(r, green, b))
	canvas.Children(func() {
		custom := s.Options.Layout == imaging.CollageCustom
		if custom && canvas.Clicked() {
			d.collage.selected = ""
			d.collage.stretch = ""
			c.Invalidate()
		}
		for i, src := range s.Items {
			if custom {
				d.drawCustomPhoto(c, src, g.Size, i, scaleX, scaleY)
				continue
			}
			cell := g.Cells[i]
			place := ui.Box(c.Key(src.Path)).Absolute().Left(float32(cell.Min.X)*scaleX).Top(float32(cell.Min.Y)*scaleY).
				Size(float32(cell.Dx())*scaleX, float32(cell.Dy())*scaleY).Clip().IgnoreInput(true)
			tile := d.collageTile(src)
			if tile.Image == nil {
				place.Background(tk.FillQuaternary)
				continue
			}
			p := s.Options.Place(cell, tile.Size)
			xs, ys := float32(p.Target.Dx())/float32(p.Source.Dx())*scaleX, float32(p.Target.Dy())/float32(p.Source.Dy())*scaleY
			place.Children(func() {
				ui.Image(c, bitmap(place, tile.Image)).Absolute().
					Left(float32(p.Target.Min.X-cell.Min.X)*scaleX-float32(p.Source.Min.X)*xs).
					Top(float32(p.Target.Min.Y-cell.Min.Y)*scaleY-float32(p.Source.Min.Y)*ys).
					Size(float32(tile.Size.X)*xs, float32(tile.Size.Y)*ys).Fit(ui.FillBox).IgnoreInput(true)
			})
		}
		if len(s.Layers) > 0 {
			marks := ui.Box(c.Key("collage-marks")).Absolute().Fill().IgnoreInput(true).ZIndex(240)
			marks.Children(func() { drawWatermarkLayers(c, marks, s.Layers, g.Size, w, h) })
		}
		ui.Box(c.Key("collage-edge")).Absolute().Fill().Radius(3).Border(.7, tk.BorderSecondary).IgnoreInput(true).ZIndex(250)
	})
}

func (d *desktop) collageTile(s files.Source) imaging.CollageTile {
	if tile, ok := d.model.Collage.Tiles[s.Path]; ok {
		return tile
	}
	if thumb := d.model.Thumbnail(s.Path); thumb.Image != nil && !s.Crop.Active() {
		return imaging.CollageTile{Image: thumb.Image, Size: image.Pt(thumb.Info.Width, thumb.Info.Height)}
	}
	return imaging.CollageTile{}
}

func (d *desktop) collageStrip(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	s, sc := &m.Collage, &d.collage.strip
	var source, target string
	ui.Row(c).Height(65).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).Basis(0).MinWidth(0).Height(65).TrackScroll(sc).Children(func() {
			ui.Row(c).Height(65).Gap(9).AlignItems(ui.Center).Children(func() {
				for i, item := range s.Items {
					key := c.Key(item.Path)
					title := trf(c, "collage.reorderImage", l10n.Args{"file": baseName(c, item.Path)})
					tile := ui.ButtonBase(key).Size(110, 61).Shrink(0).Padding(2).AlignItems(ui.Stretch).Radius(tk.BorderRadiusSM).
						Background(tk.FillQuaternary).Border(1, tk.BorderSecondary).Label(title).Tooltip(title).Clip().Disabled(s.Exporting)
					if s.Options.Layout == imaging.CollageCustom {
						if d.collage.selected == item.Path {
							tile.Border(1.6, tk.Text)
						}
						if tile.Clicked() {
							d.collage.selected = item.Path
							d.collage.stretch = ""
							c.Invalidate()
						}
					}
					if !s.Exporting {
						tile.Drag(collageDrag{item.Path}).Cursor(choose(tile.Dragging(), ui.CursorGrabbing, ui.CursorGrab))
						if v, ok := ui.DragOver[collageDrag](tile); ok && v.path != item.Path {
							tile.Border(1, tk.Primary).Background(tk.FillSecondary)
						}
						if v, ok := ui.Drop[collageDrag](tile); ok {
							source, target = v.path, item.Path
						}
						if i > 0 && tile.Shortcut(ui.Alt, ui.KeyLeft) {
							source, target = item.Path, s.Items[i-1].Path
						}
						if i+1 < len(s.Items) && tile.Shortcut(ui.Alt, ui.KeyRight) {
							source, target = item.Path, s.Items[i+1].Path
						}
					}
					tile.Children(func() {
						ui.Box(c).Width(16).Center().IgnoreInput(true).Children(func() { icon(c, lucide.GripVertical(), 12, tk.TextSecondary) })
						p := ui.Box(c).Grow(1).Basis(0).MinWidth(0).Radius(tk.BorderRadiusXS).Clip().IgnoreInput(true)
						p.Children(func() {
							if t := d.collageTile(item); t.Image != nil {
								ui.Image(c, bitmap(p, t.Image)).Fill().Fit(ui.Cover).IgnoreInput(true)
							}
							ui.Box(c).Absolute().Left(4).Top(4).Size(15, 15).Radius(4).Background(ui.RGBA(0, 0, 0, .62)).Center().IgnoreInput(true).Children(func() {
								ui.Text(c, fmt.Sprint(i+1)).FontSize(10).TextColor(ui.RGB(255, 255, 255))
							})
						})
					})
				}
			})
		})
		if len(s.Items)*119-9 > int(d.centerWidth-48) {
			if smallIcon(c, lucide.ChevronLeft(), tr(c, "collage.previous"), false, sc.X <= 0) {
				sc.X = clampScroll(sc.X-119, sc.MaxX)
			}
			if smallIcon(c, lucide.ChevronRight(), tr(c, "collage.next"), false, sc.X >= sc.MaxX) {
				sc.X = clampScroll(sc.X+119, sc.MaxX)
			}
		}
	})
	if source != "" && target != "" && source != target {
		m.MoveCollage(source, target)
		c.Invalidate()
	}
}

func (d *desktop) collageInspector(c *ui.Context) {
	m := d.model
	s := &m.Collage
	o, disabled := s.Options, s.Exporting
	h := d.editControls()
	defer h.finish()
	compactInspector(c, "", nil, func() {
		ui.Column(c).Gap(7).Children(func() {
			strong(c, tr(c, "collage.layout")).FontSize(12)
			layout := int(o.Layout)
			action.Segmented(c.Key("collage-layout"), &layout, []string{tr(c, "collage.across"), tr(c, "collage.down"), tr(c, "collage.grid"), tr(c, "collage.custom")}, action.SegmentedProps{Size: theme.Small, Block: true}).Disabled(disabled)
			o.Layout = imaging.CollageLayout(layout)
		})
		line(c)
		strong(c, tr(c, "collage.canvas"))
		two(c, func() { number(c, tr(c, "collage.width"), &o.Width, 1, imaging.MaxSide, disabled) },
			func() { number(c, tr(c, "collage.height"), &o.Height, 1, imaging.MaxSide, disabled) })
		if o.Layout != imaging.CollageCustom {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				strong(c, tr(c, "collage.gap")).FontSize(12)
				ui.Box(c).Grow(1)
				ui.Column(c).Width(80).Shrink(0).Children(func() { numberField(c, tr(c, "collage.gap"), &o.Gap, 0, 128, disabled) })
			})
		}
		h.colorPicker(c, tr(c, "collage.background"), &o.Background, disabled)
		line(c)
		if o.Layout == imaging.CollageCustom {
			if p, ok := o.Photo(d.collage.selected); ok {
				strong(c, tr(c, "collage.corners"))
				corner := int(p.Corner)
				action.Segmented(c.Key("corner-style"), &corner, []string{tr(c, "collage.square"), tr(c, "collage.round"), tr(c, "collage.bevel")}, action.SegmentedProps{Block: true, Size: theme.Small}).Disabled(disabled)
				p.Corner = imaging.CollageCorner(corner)
				if p.Corner != imaging.CornerSquare {
					number(c, tr(c, "collage.radius"), &p.Radius, 0, imaging.MaxSide/2, disabled)
				}
				o.PutPhoto(p)
			}
			return
		}
		ui.Column(c).Gap(7).Children(func() {
			strong(c, tr(c, "collage.fit")).FontSize(12)
			fit := choose(o.Fit == imaging.Cover, 1, 0)
			action.Segmented(c.Key("collage-fit"), &fit, []string{tr(c, "collage.contain"), tr(c, "collage.cover")}, action.SegmentedProps{Size: theme.Small, Block: true}).Disabled(disabled)
			o.Fit = choose(fit == 1, imaging.Cover, imaging.Contain)
		})
	})
	if o != s.Options {
		m.EditCollageOptions(o, h.group)
		c.Invalidate()
	}
}

func (d *desktop) collageStatus(c *ui.Context) {
	s := &d.model.Collage
	icon(c, lucide.Images(), 13, theme.TokensOf(c).TextSecondary)
	muted(c, trf(c, "collage.selectedCount", l10n.Args{"count": len(s.Items)})).Shrink(0)
	if s.LastOutput != "" {
		muted(c, trf(c, "collage.saved", l10n.Args{"file": filepath.Base(s.LastOutput)})).MinWidth(0).Shrink(1).Tooltip(s.LastOutput)
		if styledButtonSize(c, tr(c, "collage.openFolder"), lucide.FolderOpen(), action.Text, theme.Small, false, false).Clicked() {
			d.openOutput(filepath.Dir(s.LastOutput))
		}
	} else if s.Cancelled {
		muted(c, tr(c, "collage.cancelled"))
	}
}
