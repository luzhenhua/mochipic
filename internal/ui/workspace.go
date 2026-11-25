package ui

import (
	"fmt"
	"image"
	"math"

	"imageflow/internal/app"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (d *desktop) libraryMode(c *ui.Context) {
	current := choose(d.list, 1, 0)
	next := iconSegments(c, current, []iconSegment{{tr(c, "view.imagePreview"), lucide.Image()}, {tr(c, "view.imageList"), lucide.List()}})
	if next != current {
		d.list = next == 1
		d.model.ShowThumbnails(nil)
		d.redraw()
	}
}

func (d *desktop) viewer(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	if m.Selected == "" {
		empty(c, tr(c, "view.selectImageHint"), "", lucide.Image())
		return
	}
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Padding(4, 24, 15, 24).Children(func() {
		d.viewerToolbar(c)
		if src := m.SelectedSource(); src.Series.Multiple() {
			d.seriesControls(c.Key("series"), src)
		}
		frame := m.BeforeFrame()
		if m.Mode != app.Before {
			frame = m.AfterFrame(m.Mode != app.Encoded)
		}
		d.previewPane(c.Key("single"), frame, baseName(c, m.Selected))
		ui.Row(c).Height(33).Gap(8).AlignItems(ui.Center).Children(func() {
			info := frame.Info
			if d.cropping() {
				muted(c, tr(c, "crop.dragHint")).Grow(1).MinWidth(0).Shrink(1)
				if info.Width > 0 {
					muted(c, trf(c, "crop.originalSize", l10n.Args{"size": sizeText(info.Width, info.Height)})).Shrink(0)
				}
				return
			}
			previewMetadata(c, info)
			if info.Width > 0 && d.page == resize && m.Image.Resize.Enabled && m.Image.Resize.Mode == imaging.Exact {
				muted(c, "·")
				muted(c, fitLabel(c, m.Image.Resize.Fit))
			}
			ui.Box(c).Grow(1)
			if m.PreviewErr != nil {
				ui.Icon(c, lucide.CircleAlert()).FontSize(13).TextColor(tk.Error).Tooltip(errText(c, m.PreviewErr)).Label(errText(c, m.PreviewErr))
			} else if d.page == watermark && len(frame.Layers) != 0 {
				muted(c, tr(c, "watermark.dragHint")).MinWidth(0).Shrink(1).Tooltip(tr(c, "watermark.dragHint"))
			} else if d.page == certificationPage {
				key := "certification.dragHint"
				if frame.CertificationBounds.Empty() {
					key = "certification.selectHint"
				}
				hint := tr(c, key)
				muted(c, hint).MinWidth(0).Shrink(1).Tooltip(hint)

			}
		})
		d.scopePicker(c.Key("scope-picker"))
		d.filmstrip(c.Key("filmstrip"))
	})
}

func (d *desktop) viewerToolbar(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	ui.Row(c).Height(48).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		icon(c, lucide.FileImage(), 14, tk.TextSecondary)
		strong(c, baseName(c, m.Selected)).MinWidth(0).Shrink(1).Tooltip(m.Selected)
		ui.Box(c).Grow(1).MinWidth(0)
		if d.page == pictures && len(m.EditSelection) >= 2 {
			if button(c, tr(c, "nav.collage"), lucide.Grid2X2(), action.Default, m.Scanning) {
				d.beginCollage()
			}
		}
		if smallIcon(c, lucide.Minus(), tr(c, "view.zoomOut"), false, false) {
			d.adjustZoom(0.8)
		}
		zoomLabel := tr(c, "view.fit")
		if d.zoom == -1 {
			zoomLabel = "100%"
		} else if d.fit > 0 {
			zoomLabel = fmt.Sprintf("%.0f%%", choose(d.zoom == 0, float32(1), d.zoom)*d.fit*100)
		}
		muted(c, zoomLabel).MinWidth(32).Shrink(0).Tooltip(tr(c, "view.previewZoom"))
		if smallIcon(c, lucide.Plus(), tr(c, "view.zoomIn"), false, false) {
			d.adjustZoom(1.25)
		}
		if smallIcon(c, lucide.Maximize(), tr(c, "view.fitWindow"), d.zoom == 0, false) {
			d.setZoom(0)
		}
	})
}

func previewMetadata(c *ui.Context, info imaging.Info) {
	if info.Width <= 0 {
		return
	}
	muted(c, sizeText(info.Width, info.Height))
	muted(c, "·")
	muted(c, formatLabel(info.Format))
	if info.Bytes > 0 {
		muted(c, "·")
		muted(c, byteText(c, info.Bytes))
	}
}

func (d *desktop) previewPane(c *ui.Context, frame app.PreviewFrame, label string) {
	m, tk := d.model, theme.TokensOf(c)
	im := frame.Image
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Radius(tk.BorderRadius).Clip().Background(tk.BgLayout).Children(func() {
		viewport := ui.ScrollBoth(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Clip().TrackScroll(&d.pan)
		rect := viewport.Bounds()
		width, height := rect.W, rect.H
		if width <= 0 {
			width = d.centerWidth
		}
		if height <= 0 {
			_, windowH := c.Size()
			height = max(120, windowH-278)
		}
		pan := &d.pan
		clearLogo := false
		if !d.cropping() {
			if dx, dy, ok := viewport.Dragged(); ok && (dx != 0 || dy != 0) {
				pan.X = max(0, min(pan.MaxX, pan.X-dx))
				pan.Y = max(0, min(pan.MaxY, pan.Y-dy))
			}
			if viewport.DoubleClicked() {
				d.setZoom(choose(d.zoom == 0, float32(-1), 0))
			}
			// Watermark handles receive their own clicks. The viewport receives
			// clicks on the base image, but a pan can also report a release click.
			if d.page == watermark && d.logoSelection != "" && viewport.Clicked() {
				_, _, dragged := viewport.DragEnded()
				clearLogo = !dragged
			}
		}
		viewport.Children(func() {
			w, h := width, height
			if im != nil {
				pixels := im.Bounds().Size()
				if frame.Thumbnail && frame.Info.Width > 0 && frame.Info.Height > 0 {
					// Fit the thumbnail as if the clear preview were already here.
					scale := min(1, float64(imaging.PreviewEdge)/float64(max(frame.Info.Width, frame.Info.Height)))
					pixels = image.Pt(max(1, int(math.Round(float64(frame.Info.Width)*scale))),
						max(1, int(math.Round(float64(frame.Info.Height)*scale))))
				}
				padding := choose(d.cropping(), float32(48), 32)
				fit := min(max(1, width-padding)/float32(pixels.X), max(1, height-padding)/float32(pixels.Y))
				if math.Abs(float64(d.fit-fit)) > 0.001 {
					d.fit = fit
					c.Invalidate()
				}
				z := d.zoom
				scale := fit
				if z == -1 {
					scale = 1
				} else if z > 0 {
					scale *= z
				}
				w, h = max(1, float32(pixels.X)*scale), max(1, float32(pixels.Y)*scale)
			}
			pad := choose(im != nil, float32(32), 0)
			imgBox := ui.Box(c).Width(max(width, w+pad)).Height(max(height, h+pad)).Center()
			imgBox.Children(func() {
				if im != nil {
					drawPreviewCanvas(c, frame, w, h, label, func(canvas ui.Element) {
						switch d.page {
						case watermark:
							if clearLogo && canvas.Hovered() {
								d.logoSelection = ""
								c.Invalidate()
							}
							d.watermarkHandles(c.Key("watermark-handles"), frame, w, h)
						case certificationPage:
							d.certificationHandle(c.Key("certification-handle"), frame, w, h)
						default:
							d.cropOverlay(c, w, h)
						}
					})
				} else if m.Rendering {
					feedback.Spin(c, feedback.SpinProps{}, nil).AlignSelf(ui.Center)
				} else if m.PreviewErr != nil {
					ui.Column(c).MaxWidth(max(120, width-38)).Padding(12).Children(func() {
						feedback.Alert(c, tr(c, "view.previewFailed"), feedback.AlertProps{Type: feedback.Error, Description: errText(c, m.PreviewErr)})
					})
				} else {
					note(c, tr(c, "view.selectImageHint"))
				}
			})
		})
	})
}

func drawPreviewFrame(c *ui.Context, frame app.PreviewFrame, w, h float32, label string) {
	drawPreviewCanvas(c, frame, w, h, label, nil)
}

func drawPreviewCanvas(c *ui.Context, frame app.PreviewFrame, w, h float32, label string, overlay func(ui.Element)) {
	canvas := ui.Box(c).Size(w, h).Radius(4).Clip()
	if r, g, b, err := imaging.ParseColor(frame.Background); err == nil {
		canvas.Background(ui.RGB(r, g, b))
	}
	canvas.Children(func() {
		display.Image(c.Key("base-image"), bitmap(canvas, frame.Image), display.ImageProps{
			Width: w, Height: h, Contain: true, NoPreview: true, Alt: label, Variant: display.ImageBorderless,
		}).Radius(0)
		if overlay != nil {
			overlay(canvas)
		}
		drawWatermarkLayers(c.Key("marks"), canvas, frame.Layers, image.Pt(frame.Info.Width, frame.Info.Height), w, h)
	})
}

func drawWatermarkLayers(c *ui.Context, canvas ui.Element, layers []imaging.PreviewLayer, size image.Point, w, h float32) {
	cache := ui.Local(canvas, "watermark-bitmaps", func() map[image.Image]*ui.Bitmap { return nil })
	if len(layers) == 0 || size.X <= 0 || size.Y <= 0 {
		*cache = nil
		return
	}
	sx, sy := w/float32(size.X), h/float32(size.Y)
	next := make(map[image.Image]*ui.Bitmap, len(layers))
	for i, layer := range layers {
		b := layer.Bounds
		x, y, width, height := float32(b.Min.X)*sx, float32(b.Min.Y)*sy, float32(b.Dx())*sx, float32(b.Dy())*sy
		if layer.Pixels == nil {
			f := layer.Fill
			ui.Box(c.Key(i)).Absolute().Left(x).Top(y).Size(width, height).
				Background(ui.RGBA(f.R, f.G, f.B, float32(f.A)/255)).ZIndex(i + 1).IgnoreInput(true)
			continue
		}
		tex := next[layer.Pixels]
		if tex == nil {
			tex = (*cache)[layer.Pixels]
			if tex == nil {
				tex = ui.NewBitmap(layer.Pixels)
			}
			next[layer.Pixels] = tex
		}
		ui.Image(c.Key(i), tex).Absolute().Left(x).Top(y).Size(width, height).
			Fit(ui.FillBox).Opacity(float32(layer.Opacity) / 100).
			ZIndex(i + 1).IgnoreInput(true)
	}
	*cache = next
}

type bitmapSlot struct {
	image  image.Image
	bitmap *ui.Bitmap
}

func bitmap(e ui.Element, im image.Image) *ui.Bitmap {
	slot := ui.Local(e, "imageflow-bitmap", func() bitmapSlot { return bitmapSlot{} })
	if slot.image != im {
		slot.image = im
		if slot.bitmap == nil {
			slot.bitmap = ui.NewBitmap(im)
		} else {
			slot.bitmap.Update(im)
		}
	}
	return slot.bitmap
}

func (d *desktop) setZoom(z float32) {
	if d.page == collage {
		d.collage.zoom = z
		d.collage.pan.X, d.collage.pan.Y = 0, 0
		d.redraw()
		return
	}
	d.zoom = z
	d.pan.X, d.pan.Y = 0, 0
	d.redraw()
}

func (d *desktop) adjustZoom(k float32) {
	z := d.zoom
	if z == -1 {
		z = 1 / max(0.01, d.fit)
	} else if z == 0 {
		z = 1
	}
	d.setZoom(max(0.35, min(10, z*k)))
}

func (d *desktop) cropStrip() bool { return d.page == resize && d.resizeTab == 0 }

func (d *desktop) thumbSlots() int {
	if d.cropStrip() {
		return min(8, max(1, int((d.centerWidth-108)/118)))
	}
	return min(8, max(3, int((d.centerWidth-35)/82)))
}

func (d *desktop) thumb(c *ui.Context, path string, w, h float32) {
	d.thumbFit(c, path, w, h, true)
}

func (d *desktop) thumbFit(c *ui.Context, path string, w, h float32, contain bool) {
	t := d.model.Thumbnail(path)
	box := ui.Box(c).Height(h).MinWidth(0).AlignSelf(ui.Stretch)
	if w > 0 {
		box.Width(w).Shrink(0)
	}
	var imageSource ui.ImageSource
	if t.Image != nil {
		imageSource = bitmap(box, t.Image)
	}
	p := display.ImageProps{Width: w, Height: h, Contain: contain, NoPreview: true, Failed: t.Err != nil, Loading: t.Image == nil && t.Err == nil, Alt: baseName(c, path)}
	box.Children(func() {
		display.Image(c, imageSource, p).Height(h).MinWidth(0).AlignSelf(ui.Stretch).Radius(5).Background(theme.TokensOf(c).FillSecondary)
	})
}

func pathsOf(items []files.Source) []string {
	paths := make([]string, len(items))
	for i, s := range items {
		paths[i] = s.Path
	}
	return paths
}

func (d *desktop) imageInspector(c *ui.Context) {
	m := d.model
	compactInspector(c, tr(c, "view.processingSettings"), nil, func() {
		size := resizeText(c, m.Image.Resize)
		if src := m.SelectedSource(); src.Crop.Active() && !m.Image.Resize.Enabled && m.OriginalReady() {
			box := src.Crop.Resolve(image.Pt(m.Preview.Input.Width, m.Preview.Input.Height))
			size = sizeText(box.Dx(), box.Dy())
		}
		ui.Column(c).Children(func() {
			d.paramRow(c, watermark, tr(c, "nav.watermark"), markText(c, m.Image), lucide.Stamp())
			badges := m.Certification().Count()
			value := choose(badges == 0, tr(c, "certification.none"), trf(c, "certification.badgeCount", l10n.Args{"count": badges}))
			d.paramRow(c, certificationPage, tr(c, "nav.certification"), value, lucide.BadgeCheck())
			d.paramRow(c, captionPage, tr(c, "nav.caption"), choose(m.Caption().Active(), tr(c, "caption.enabled"), tr(c, "watermark.off")), lucide.PanelBottom())
			d.paramRow(c, resize, tr(c, "nav.resize"), size, lucide.Scaling())
			d.paramRow(c, format, tr(c, "nav.format"), formatText(c, m.Image), lucide.FileImage())
			d.paramRow(c, rename, tr(c, "nav.rename"), choose(m.Naming.KeepOriginal, tr(c, "rename.keepOriginal"), tr(c, "rename.customTemplate")), lucide.TextCursorInput())
		})
		line(c)
		strong(c, tr(c, "output.file"))
		pathLine(c, d.outputName(c), lucide.FileImage())
		if e, ok := m.CurrentEntry(); ok && e.Issue != nil {
			note(c, errText(c, e.Issue))
		}
	})
}

func (d *desktop) outputName(c *ui.Context) string {
	if e, ok := d.model.CurrentEntry(); ok && e.Issue == nil {
		return choose(e.Skip, trf(c, "output.skipName", l10n.Args{"name": baseName(c, e.Relative)}), baseName(c, e.Relative))
	}
	return ""
}

func (d *desktop) paramRow(c *ui.Context, p page, title, value string, svg *ui.SVG) {
	tk := theme.TokensOf(c)
	b := ui.ButtonBase(c.Key(p)).Height(41).Label(title).AlignItems(ui.Center).Gap(7).PaddingX(2).Radius(tk.BorderRadiusSM)
	if b.Hovered() {
		b.Background(tk.FillQuaternary)
	}
	b.Children(func() {
		icon(c, svg, 15, tk.TextSecondary)
		ui.Text(c, title).FontSize(13).SingleLine().Shrink(0)
		ui.Box(c).Grow(1).MinWidth(0)
		muted(c, value).MinWidth(0).Shrink(1).Tooltip(value)
		icon(c, lucide.ChevronRight(), 13, tk.TextTertiary)
	})
	if b.Clicked() {
		d.navigate(p)
	}
}
