package ui

import (
	"fmt"
	"image"

	"imageflow/internal/app"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func markText(c *ui.Context, o imaging.Options) string {
	logos := len(o.LogoMarks()) > 0
	switch {
	case o.Text.Enabled && logos:
		return tr(c, "watermark.both")
	case o.Text.Enabled:
		return tr(c, "watermark.text")
	case logos:
		return tr(c, "watermark.logo")
	default:
		return tr(c, "watermark.off")
	}
}

func resizeText(c *ui.Context, r imaging.Resize) string {
	if !r.Enabled {
		return tr(c, "resize.originalSize")
	}
	switch r.Mode {
	case imaging.Percent:
		return trf(c, "resize.scaleSummary", l10n.Args{"percent": r.Percent})
	case imaging.LongEdge:
		return trf(c, "resize.longEdgeSummary", l10n.Args{"pixels": r.Limit})
	default:
		return sizeText(r.Width, r.Height)
	}
}

func formatLabel(f imaging.Format) string {
	if f == "" {
		return "—"
	}
	return string(f)
}

func fitLabel(c *ui.Context, f imaging.Fit) string {
	switch f {
	case imaging.Cover:
		return tr(c, "resize.cover")
	case imaging.Stretch:
		return tr(c, "resize.stretch")
	default:
		return tr(c, "resize.contain")
	}
}

func formatText(c *ui.Context, o imaging.Options) string {
	if o.Format == imaging.PNG || o.Format == imaging.WebP && o.Lossless {
		return trf(c, "format.losslessSummary", l10n.Args{"format": formatLabel(o.Format)})
	}
	return fmt.Sprintf("%s · %d%%", o.Format, o.Quality)
}

func (d *desktop) watermarkInspector(c *ui.Context) {
	m := d.model
	o := m.Image
	before := o
	h := d.editControls()
	defer h.finish()
	compactInspector(c, "", nil, func() {
		action.Segmented(c.Key("watermark-tabs"), &d.watermarkTab, []string{tr(c, "watermark.textTab"), tr(c, "watermark.logoTab"), tr(c, "watermark.layers")}, action.SegmentedProps{Size: theme.Small, Block: true})
		ui.Column(c.Key(d.watermarkTab)).Gap(10).Children(func() {
			switch d.watermarkTab {
			case 0:
				d.textControls(c, &before.Text, &o.Text, &h)
			case 1:
				d.logoControls(c, &before, &o, &h)
			default:
				d.layersControls(c, &o)
			}
		})
	})
	if before != o {
		label := [...]app.HistoryAction{app.HistoryText, app.HistoryLogo, app.HistoryLayers}
		m.EditImageFrom(before, o, label[d.watermarkTab], h.group)
		// The viewer is built before this inspector. Paint the new layer
		// properties on the next frame without waiting for a worker.
		c.Invalidate()
	}
}

func (d *desktop) textControls(c *ui.Context, before, t *imaging.TextMark, h *historyControls) {
	hint := tr(c, "watermark.textPlaceholder")
	if d.hasMixed("watermark.textContent") {
		hint = tr(c, "scope.mixed")
		before.Text, t.Text = "", ""
	}
	prev := t.Text
	text := input(c, tr(c, "watermark.textContent"), hint, &t.Text, false)
	h.track(text, text.Focused(), t.Text != prev)
	two(c, func() {
		strong(c, tr(c, "watermark.font")).FontSize(12)
		if d.hasMixed("watermark.font") {
			before.Font, t.Font = "", ""
		}
		fonts := []field.SelectOption{
			{Value: "Helvetica Neue Bold", Label: tr(c, "font.helveticaBold")}, {Value: "Arial Bold", Label: tr(c, "font.arialBold")}, {Value: "Arial", Label: "Arial"},
			{Value: "PingFang SC", Label: tr(c, "font.pingfang")}, {Value: "Songti SC", Label: tr(c, "font.songti")}, {Value: "Menlo", Label: "Menlo"},
		}
		for i := range fonts {
			if name := imaging.InstalledWatermarkFont(fonts[i].Value); name != fonts[i].Value {
				fonts[i].Label = name
			}
		}
		field.Select(c, &t.Font, field.SelectProps{Placeholder: tr(c, "scope.mixed"), Options: fonts}).
			Label(tr(c, "watermark.font")).Tooltip(imaging.InstalledWatermarkFont(t.Font)).AlignSelf(ui.Stretch)
	}, func() { d.scopeNumber(c, "watermark.fontSize", &before.Size, &t.Size, 1, 512, false) })
	h.colorPicker(c, tr(c, "watermark.color"), &t.Color, false)
	h.slider(c, "watermark.opacity", &before.Opacity, &t.Opacity, 0, 100, false)
	h.slider(c, "watermark.scale", &before.Scale, &t.Scale, 1, 200, false)
	line(c)
	placement(c, &t.Placement)
	ui.Row(c).Gap(16).Children(func() {
		choice.Checkbox(c, &t.Stroke, tr(c, "watermark.stroke"), choice.CheckboxProps{})
		choice.Checkbox(c, &t.Shadow, tr(c, "watermark.shadow"), choice.CheckboxProps{})
	})
}

var anchors = [...]string{"anchor.topLeft", "anchor.topCenter", "anchor.topRight", "anchor.centerLeft", "anchor.center", "anchor.centerRight", "anchor.bottomLeft", "anchor.bottomCenter", "anchor.bottomRight"}

func anchorLabel(c *ui.Context, i int) string { return tr(c, anchors[max(0, min(len(anchors)-1, i))]) }

func placementLabel(c *ui.Context, p imaging.Placement) string {
	if p.Free.Active {
		return tr(c, "watermark.freePosition")
	}
	return anchorLabel(c, p.Anchor)
}

func placement(c *ui.Context, p *imaging.Placement) {
	anchorControls(c, p, tr(c, choose(p.Free.Active, "watermark.dragHint", "watermark.marginHint")))
	if !p.Free.Active {
		two(c, func() { number(c, tr(c, "watermark.marginX"), &p.MarginX, 0, imaging.MaxSide, false) },
			func() { number(c, tr(c, "watermark.marginY"), &p.MarginY, 0, imaging.MaxSide, false) })
	}
}

func anchorControls(c *ui.Context, p *imaging.Placement, hint string) {
	tk := theme.TokensOf(c)
	ui.Column(c).Gap(5).Children(func() {
		strong(c, tr(c, "common.location")).FontSize(12)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(2).Padding(2).Radius(tk.BorderRadiusSM).Background(tk.BgLayout).Border(1, tk.BorderSecondary).Children(func() {
				for row := range 3 {
					ui.Row(c.Key(row)).Gap(2).Children(func() {
						for col := range 3 {
							idx := row*3 + col
							b := ui.ButtonBase(c.Key(idx)).Size(25, 20).Center().Radius(3).Label(anchorLabel(c, idx)).Tooltip(anchorLabel(c, idx))
							selected := !p.Free.Active && idx == p.Anchor
							switch {
							case selected:
								b.Background(tk.Primary)
							case b.Hovered():
								b.Background(tk.FillSecondary)
							default:
								b.Background(tk.BgContainer)
							}
							b.Children(func() {
								ui.Box(c).Size(4, 4).Radius(2).Background(choose(selected, tk.BgContainer, tk.TextQuaternary))
							})
							if b.Clicked() {
								p.Anchor = idx
								p.Free = imaging.FreePosition{}
							}
						}
					})
				}
			})
			muted(c, placementLabel(c, *p)).Tooltip(hint)
		})
	})
}

func (d *desktop) resizeInspector(c *ui.Context) {
	m := d.model
	o := m.Image
	before := o
	r := &o.Resize
	h := d.editControls()
	defer h.finish()
	compactInspector(c, tr(c, "resize.title"), func() {
		choice.Switch(c, &r.Enabled, choice.SwitchProps{}).Label(tr(c, "resize.enable"))
	}, func() {
		disabled := !r.Enabled
		modes := [...]imaging.ResizeMode{imaging.Exact, imaging.Percent, imaging.LongEdge}
		v := 0
		for i, x := range modes {
			if x == r.Mode {
				v = i
			}
		}
		ui.Column(c).Gap(5).Children(func() {
			strong(c, tr(c, "common.mode")).FontSize(12)
			action.Segmented(c, &v, []string{tr(c, "resize.exactMode"), tr(c, "resize.percentMode"), tr(c, "resize.longEdgeMode")}, action.SegmentedProps{Size: theme.Small, Block: true}).Disabled(disabled)
		})
		if r.Enabled {
			r.Mode = modes[v]
		}
		switch r.Mode {
		case imaging.Exact:
			two(c, func() { d.scopeNumber(c, "resize.width", &before.Resize.Width, &r.Width, 1, imaging.MaxSide, disabled) }, func() {
				d.scopeNumber(c, "resize.height", &before.Resize.Height, &r.Height, 1, imaging.MaxSide, disabled)
			})
			switchRow(c, tr(c, "resize.lockRatio"), nil, &r.LockRatio, disabled)
			line(c)
			ui.Column(c).Gap(2).Children(func() {
				strong(c, tr(c, "resize.fitMode")).Margin(0, 0, 4, 0)
				radioOption(c.Key(imaging.Contain), &r.Fit, imaging.Contain, tr(c, "resize.contain"), tr(c, "resize.containHint"), disabled)
				radioOption(c.Key(imaging.Cover), &r.Fit, imaging.Cover, tr(c, "resize.cover"), tr(c, "resize.coverHint"), disabled)
				radioOption(c.Key(imaging.Stretch), &r.Fit, imaging.Stretch, tr(c, "resize.stretch"), tr(c, "resize.stretchHint"), disabled)
			})
			if r.Fit == imaging.Contain {
				h.colorPicker(c, tr(c, "resize.paddingColor"), &r.PadColor, disabled || r.Transparent)
				choice.Checkbox(c, &r.Transparent, tr(c, "resize.transparentPadding"), choice.CheckboxProps{}).Disabled(disabled)
			}
			if r.Fit == imaging.Stretch && r.Enabled {
				feedback.Alert(c, tr(c, "resize.stretchWarning"), feedback.AlertProps{Type: feedback.Warning})
			}
		case imaging.Percent:
			h.slider(c, "resize.scale", &before.Resize.Percent, &r.Percent, 1, 400, disabled)
		case imaging.LongEdge:
			d.scopeNumber(c, "resize.longEdgeMax", &before.Resize.Limit, &r.Limit, 1, imaging.MaxSide, disabled)
			note(c, tr(c, "resize.longEdgeHint"))
		}
		line(c)
		in := m.Preview.Input
		ui.Column(c).Children(func() {
			detailRow(c, tr(c, "resize.originalSize"), sizeText(in.Width, in.Height))
			if in.Width == 0 {
				return
			}
			box := m.SelectedSource().Crop.Resolve(image.Pt(in.Width, in.Height))
			if box.Size() != (image.Pt(in.Width, in.Height)) {
				detailRow(c, tr(c, "crop.resultSize"), sizeText(box.Dx(), box.Dy()))
			}
			g, err := r.Geometry(box.Dx(), box.Dy())
			if err != nil {
				note(c, errText(c, err))
				return
			}
			if g.Width > g.ImageWidth {
				detailRow(c, tr(c, "resize.horizontalPadding"), paddingText(c, g.X, g.Width-g.ImageWidth-g.X))
			}
			if g.Height > g.ImageHeight {
				detailRow(c, tr(c, "resize.verticalPadding"), paddingText(c, g.Y, g.Height-g.ImageHeight-g.Y))
			}
			detailRow(c, tr(c, "resize.outputSize"), sizeText(g.Width, g.Height))
		})
	})
	m.EditImageFrom(before, o, app.HistoryResize, h.group)
}

func paddingText(c *ui.Context, a, b int) string {
	if a == b {
		return trf(c, "resize.equalPadding", l10n.Args{"pixels": a})
	}
	return fmt.Sprintf("%d / %d px", a, b)
}

var formatOptions, formatLabels = func() ([]field.SelectOption, []string) {
	var options []field.SelectOption
	var labels []string
	for _, f := range imaging.OutputFormats() {
		label := formatLabel(f)
		options = append(options, field.SelectOption{Value: string(f), Label: label})
		labels = append(labels, label)
	}
	return options, labels
}()

func (d *desktop) formatInspector(c *ui.Context) {
	m := d.model
	o := m.Image
	before := o
	h := d.editControls()
	defer h.finish()
	compactInspector(c, tr(c, "format.outputFormat"), nil, func() {
		if d.hasMixed("format.outputFormat") {
			if idx, ok := mixedChoice(c.Key("format"), tr(c, "format.outputFormat"), formatLabels); ok {
				before.Format = ""
				o.Format = imaging.Format(formatOptions[idx].Value)
			}
		} else {
			v := 0
			for i, f := range formatOptions {
				if f.Value == string(o.Format) {
					v = i
					break
				}
			}
			action.Segmented(c.Key("format"), &v, formatLabels, action.SegmentedProps{Size: theme.Small, Block: true})
			o.Format = imaging.Format(formatOptions[v].Value)
		}
		if o.Format == imaging.WebP {
			ui.Column(c).Gap(5).Children(func() {
				strong(c, tr(c, "format.webpEncoding")).FontSize(12)
				i := choose(o.Lossless, 1, 0)
				action.Segmented(c.Key("webp-mode"), &i, []string{tr(c, "format.lossy"), tr(c, "format.lossless")}, action.SegmentedProps{Size: theme.Small, Block: true})
				o.Lossless = i == 1
			})
		}
		if o.UsesQuality() {
			h.slider(c, "format.quality", &before.Quality, &o.Quality, 1, 100, false)
		}
		line(c)
		jpg := o.Format == imaging.JPEG
		ui.Column(c).Gap(5).Children(func() {
			strong(c, tr(c, "format.transparency")).FontSize(12)
			alpha := choose(jpg || o.FlattenAlpha, "fill", "keep")
			field.Select(c, &alpha, field.SelectProps{Disabled: jpg, Options: []field.SelectOption{
				{Value: "keep", Label: tr(c, "format.keepAlpha")}, {Value: "fill", Label: tr(c, "format.fillAlpha")},
			}}).Label(tr(c, "format.transparency")).AlignSelf(ui.Stretch)
			if !jpg {
				o.FlattenAlpha = alpha == "fill"
			}
		})
		if jpg || o.FlattenAlpha {
			h.colorPicker(c, tr(c, choose(jpg, "format.jpgBackground", "format.backgroundColor")), &o.Background, false)
		}
		line(c)
		in, out := m.Preview.Input, m.Preview.Output
		ui.Column(c).Children(func() {
			detailRow(c, tr(c, "format.originalBytes"), choose(in.Bytes > 0, byteText(c, in.Bytes), "—"))
			detailRow(c, tr(c, "format.outputBytes"), choose(out.Bytes > 0, byteText(c, out.Bytes), "—"))
		})
	})
	m.EditImageFrom(before, o, app.HistoryFormat, h.group)
}

func entryTag(c *ui.Context, issue error, skip, changed bool) {
	switch {
	case issue != nil:
		tag(c, tr(c, "output.entryPlanError"), display.TagError)
	case skip:
		tag(c, tr(c, "output.willSkip"), display.TagWarning)
	case changed:
		tag(c, tr(c, "output.autoRenamed"), "")
	}
}
