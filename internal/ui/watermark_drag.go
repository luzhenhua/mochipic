package ui

import (
	"image"
	"math"

	"imageflow/internal/app"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo/ui"
)

type markDrag struct {
	held   bool
	origin image.Point
	dx, dy float64
	group  app.HistoryGroup
	model  *app.Model
}

func (d *markDrag) Close() error {
	d.model.CancelHistoryGroup(d.group)
	return nil
}

func (d *desktop) watermarkHandles(c *ui.Context, frame app.PreviewFrame, width, height float32) {
	m := d.model
	size := image.Pt(frame.Info.Width, frame.Info.Height)
	if size.X <= 0 || size.Y <= 0 || width <= 0 || height <= 0 || m.Working || m.Rendering && !m.OriginalReady() {
		return
	}
	for i, layer := range frame.Layers {
		if layer.Mark == 0 || layer.Bounds.Empty() {
			continue
		}
		key := struct {
			Path, Logo string
			Part       int
			Mark       imaging.WatermarkKind
		}{m.Selected, layer.LogoPath, m.PreviewPart, layer.Mark}
		label := tr(c, choose(layer.Mark == imaging.TextWatermark, "watermark.dragText", "watermark.dragLogo"))
		var selectLogo func()
		var selected bool
		if layer.Mark == imaging.LogoWatermark {
			if len(m.Image.LogoMarks()) > 1 {
				label = trf(c, "watermark.dragNamedLogo", l10n.Args{"file": baseName(c, layer.LogoPath)})
			}
			selectLogo = func() {
				if d.watermarkTab != 1 || d.logoSelection != layer.LogoPath {
					d.watermarkTab = 1
					d.selectLogo(layer.LogoPath, m.Image.LogoMarks())
					c.Invalidate()
				}
			}
			selected = d.watermarkTab == 1 && d.logoSelection == layer.LogoPath
		}
		markHandle(c.Key(key), m, layer.Bounds, size, width, height, len(frame.Layers)+i+1, label, selected, selectLogo, func(pos image.Point, group app.HistoryGroup) {
			d.moveWatermark(c, layer, pos, size, group)
		})
	}
}

func markHandle(c *ui.Context, m *app.Model, b image.Rectangle, size image.Point, width, height float32, z int, label string, selected bool, selectMark func(), move func(image.Point, app.HistoryGroup)) {
	if size.X <= 0 || size.Y <= 0 || width <= 0 || height <= 0 {
		return
	}
	sx, sy := width/float32(size.X), height/float32(size.Y)
	w, h := float32(b.Dx())*sx, float32(b.Dy())*sy
	hitW, hitH := min(width, max(16, w)), min(height, max(16, h))
	x := max(0, min(width-hitW, float32(b.Min.X)*sx-(hitW-w)/2))
	y := max(0, min(height-hitH, float32(b.Min.Y)*sy-(hitH-h)/2))
	handle := ui.ButtonBase(c).Absolute().Left(x).Top(y).Size(hitW, hitH).
		ZIndex(z).Label(label).Tooltip(label).Cursor(ui.CursorMove)
	if selectMark != nil {
		if selected {
			handle.Checked(true)
		}
		if handle.Pressed() || handle.Clicked() {
			selectMark()
		}
	}
	drag := ui.Local(handle, "mark-drag", func() markDrag { return markDrag{model: m} })
	dx, dy, held := handle.Dragged()
	if held {
		if !drag.held {
			*drag = markDrag{held: true, origin: b.Min, group: m.NewHistoryGroup(), model: m}
			if selectMark != nil {
				selectMark()
			}
		}
		drag.dx += float64(dx / sx)
		drag.dy += float64(dy / sy)
		if dx != 0 || dy != 0 {
			move(drag.origin.Add(image.Pt(int(math.Round(drag.dx)), int(math.Round(drag.dy)))), drag.group)
		}
	} else {
		if drag.held {
			if handle.PressCanceled() {
				m.CancelHistoryGroup(drag.group)
			} else {
				m.FinishHistoryGroup(drag.group)
			}
		}
		*drag = markDrag{model: m}
		if handle.Focused() {
			for _, k := range []struct {
				key ui.Key
				d   image.Point
			}{{ui.KeyLeft, image.Pt(-1, 0)}, {ui.KeyRight, image.Pt(1, 0)}, {ui.KeyUp, image.Pt(0, -1)}, {ui.KeyDown, image.Pt(0, 1)}} {
				if handle.Shortcut(0, k.key) {
					if selectMark != nil {
						selectMark()
					}
					move(b.Min.Add(k.d), 0)
				}
			}
		}
	}
	if selected || held || selectMark == nil && (handle.Hovered() || handle.Focused()) {
		handle.Radius(3).Border(1, ui.RGBA(255, 255, 255, .85)).
			InsetShadow(0, 0, 0, 1.8, ui.RGBA(0, 0, 0, .42))
	}
}

func (d *desktop) moveWatermark(c *ui.Context, layer imaging.PreviewLayer, origin, canvas image.Point, group app.HistoryGroup) {
	o := d.model.Image
	label := app.HistoryText
	p := &o.Text.Placement
	if layer.Mark == imaging.LogoWatermark {
		logo := o.LogoByPath(layer.LogoPath)
		if logo == nil {
			return
		}
		p = &logo.Placement
		label = app.HistoryLogo
	}
	p.MoveTo(origin, canvas, layer.Bounds.Size())
	if o != d.model.Image {
		d.model.EditImage(o, label, group)
		c.Invalidate()
	}
}
