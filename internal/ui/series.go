package ui

import (
	"math"
	"strconv"

	"imageflow/internal/files"
	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (d *desktop) seriesControls(c *ui.Context, src files.Source) {
	frames := src.Series.Kind == series.Frames
	m := d.model
	s := src.Selection
	h := d.editControls()
	defer h.finish()
	problem := s.Validate(src.Series)
	prompt := d.seriesPrompt == src.Path && problem != nil
	if d.seriesPrompt == src.Path && !prompt {
		d.seriesPrompt = ""
	}
	pos := max(1, m.PreviewPart)
	ui.Column(c).Shrink(0).MinWidth(0).Gap(5).Margin(0, 0, 8, 0).Children(func() {
		ui.Row(c).MinHeight(34).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
			key := choose(frames, "series.frame", "series.page")
			if smallIcon(c.Key("previous-part"), lucide.ChevronLeft(), tr(c, choose(frames, "series.previousFrame", "series.previousPage")), false, pos == 1) {
				m.SetPreviewPart(pos - 1)
			}
			muted(c, tr(c, key)).FontSize(12)
			jump := float64(pos)
			field.InputNumber(c.Key("preview-part"), &jump, field.InputNumberProps{Min: 1, Max: float64(src.Series.Count), NoControls: true, Size: theme.Small}).
				Width(49).Label(tr(c, choose(frames, "series.previewFrame", "series.previewPage")))
			if jump >= 1 && jump <= float64(src.Series.Count) && math.Trunc(jump) == jump && int(jump) != pos {
				m.SetPreviewPart(int(jump))
			}
			muted(c, "/ "+strconv.Itoa(src.Series.Count)).FontSize(12)
			if smallIcon(c.Key("next-part"), lucide.ChevronRight(), tr(c, choose(frames, "series.nextFrame", "series.nextPage")), false, pos == src.Series.Count) {
				m.SetPreviewPart(pos + 1)
			}
			ui.Box(c).Grow(1).MinWidth(0)
			muted(c, tr(c, "series.scope")).FontSize(12)
			for _, option := range []struct {
				scope series.Scope
				label string
			}{
				{series.All, choose(frames, "series.allFrames", "series.allPages")},
				{series.Custom, choose(frames, "series.customFrames", "series.customPages")},
			} {
				active := s.Scope == option.scope
				b := action.Button(c.Key(option.scope), tr(c, option.label), action.ButtonProps{
					Size: theme.Small, Variant: choose(active, action.Filled, action.Default),
				}).AccessSelected(active)
				if b.Clicked() {
					s.Scope = option.scope
				}
			}
		})
		if s.Scope != "" {
			ui.Row(c).MinHeight(29).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
				if s.Scope == series.Custom {
					label := tr(c, "series.range")
					muted(c, label).FontSize(12)
					edit := field.Input(c.Key("series-range"), &s.Range, field.InputProps{Size: theme.Small, Placeholder: "1, 3-5", Invalid: prompt}).Width(104).Label(label)
					if src.Selection.Scope != series.Custom {
						edit.Focus()
					}
					h.track(edit, edit.Focused(), src.Selection.Range != s.Range)
				}
				ui.Box(c).Grow(1).MinWidth(0)
				muted(c, tr(c, "series.output")).FontSize(12)
				output := choose(s.Separate, "separate", "combined")
				field.Select(c.Key("series-output"), &output, field.SelectProps{Size: theme.Small, Options: []field.SelectOption{
					{Value: "combined", Label: tr(c, choose(frames, "series.combineWebP", "series.combineTIFF"))},
					{Value: "separate", Label: trf(c, "series.separate", l10n.Args{"format": formatLabel(m.Image.Format)})},
				}}).MinWidth(145).Label(tr(c, "series.output"))
				s.Separate = output == "separate"
			})
		}
		if prompt || s.Scope == series.Custom && s.Range != "" && problem != nil {
			ui.Text(c, errText(c, problem)).FontSize(12).Wrap().TextColor(theme.TokensOf(c).ErrorText)
		}
	})
	m.EditSeriesSelection(s, h.group)
}

func sourceName(c *ui.Context, s files.Source) string {
	name := baseName(c, s.Path)
	if s.Part == 0 {
		return name
	}
	key := choose(s.Series.Kind == series.Frames, "series.sourceFrame", "series.sourcePage")
	return trf(c, key, l10n.Args{"name": name, "part": s.Part})
}
