package ui

import (
	"imageflow/internal/caption"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type captionControlsState struct {
	saved caption.Style
}

func (d *desktop) captionInspector(c *ui.Context) {
	m := d.model
	value := m.Caption()
	previous := value
	hint := tr(c, "caption.placeholder")
	if d.hasMixed("caption.title") {
		hint = tr(c, "scope.mixed")
		previous.Text, value.Text = "", ""
	}
	h := d.editControls()
	defer h.finish()
	label := tr(c, "caption.title")
	compactInspector(c, tr(c, "caption.strip"), nil, func() {
		body := ui.Column(c.Key(m.Selected)).Gap(12)
		state := ui.Local(body, "controls", func() captionControlsState { return captionControlsState{} })
		body.Children(func() {
			ui.Column(c.Key("title")).Gap(5).Children(func() {
				strong(c, label).FontSize(12)
				edit := field.TextArea(c, &value.Text, field.TextAreaProps{
					MinRows: 3, MaxRows: 3, Placeholder: hint,
				}).Label(label).MaxLength(caption.MaxRunes).FontSize(13)
				value.Text = caption.CleanText(value.Text)
				h.track(edit, edit.Focused(), value.Text != previous.Text)
				ui.Text(c, tr(c, "caption.wrapHint")).Wrap().FontSize(11).TextColor(theme.TokensOf(c).TextSecondary)
			})
			h.colorPicker(c.Key("background"), tr(c, "caption.background"), &value.Style.Background, false)
			h.slider(c.Key("opacity"), "caption.opacity", &previous.Style.Opacity, &value.Style.Opacity, 0, 100, false)
			captionHeightField(c.Key("height"), &previous.Style, &value.Style, &h)
			line(c)
			if captionDisclosure(c.Key("advanced")) {
				captionTextFields(c.Key("typography"), &previous.Style, &value.Style, &h)
			}
			line(c)
			if value != previous {
				if value.Style != previous.Style {
					state.saved = caption.Style{}
				}
				m.EditCaptionFrom(previous, value, h.group)
				c.Invalidate()
			}
			ui.Row(c.Key("actions")).AlignItems(ui.Center).Wrap().Gap(6).Children(func() {
				if styledButtonSize(c.Key("save"), tr(c, "caption.saveStyle"), lucide.Bookmark(), action.Text, theme.Small,
					!m.CanSaveCaptionStyle(value.Style), false).Tooltip(tr(c, "caption.saveHint")).Clicked() {
					m.SaveCaptionStyle(value.Style)
					state.saved = value.Style
					c.Invalidate()
				}
			})
			if state.saved != (caption.Style{}) && state.saved == value.Style && !m.PrefsPending {
				if m.PreferenceError != nil {
					note(c, errText(c, m.PreferenceError))
				} else {
					note(c, tr(c, "caption.styleSaved"))
				}
			}
		})
	})
}

func captionHeightField(c *ui.Context, before, s *caption.Style, h *historyControls) {
	mode := choose(s.Height == 0, "auto", "fixed")
	ui.Column(c).Gap(5).Children(func() {
		label := tr(c, "caption.height")
		strong(c, label).FontSize(12)
		if h.hasMixed("caption.height") {
			mode = ""
		}
		field.Select(c, &mode, field.SelectProps{Placeholder: tr(c, "scope.mixed"), Options: []field.SelectOption{
			{Value: "auto", Label: tr(c, "caption.fitTitle")}, {Value: "fixed", Label: tr(c, "caption.customHeight")},
		}}).Label(label).AlignSelf(ui.Stretch)
		if mode != "" {
			if h.hasMixed("caption.height") {
				before.Height = -1
			}
			if mode == "fixed" && s.Height == 0 {
				s.Height = caption.ManualHeight
			} else if mode == "auto" {
				s.Height = 0
			}
		}
		if s.Height != 0 {
			h.slider(c, "caption.heightPercent", &before.Height, &s.Height, caption.MinHeight, caption.MaxHeight, false)
		}
	})
}

func captionDisclosure(c *ui.Context) bool {
	tk := theme.TokensOf(c)
	label := tr(c, "caption.more")
	row := ui.ButtonBase(c).Height(26).AlignItems(ui.Center).Gap(8).Radius(tk.BorderRadiusSM).
		Label(label).Role(ui.RoleToggleButton)
	open := ui.Local(row, "expanded", func() bool { return false })
	if row.Clicked() {
		*open = !*open
	}
	row.Checked(*open)
	if row.Hovered() {
		row.Background(tk.FillQuaternary)
	}
	row.Children(func() {
		icon(c, lucide.SlidersHorizontal(), 14, tk.TextSecondary)
		muted(c, label)
		ui.Box(c).Grow(1)
		icon(c, choose(*open, lucide.ChevronUp(), lucide.ChevronDown()), 13, tk.TextSecondary)
	})
	return *open
}

func captionTextFields(c *ui.Context, before, s *caption.Style, h *historyControls) {
	ui.Column(c).Gap(10).Children(func() {
		h.slider(c.Key("size"), "caption.size", &before.Size, &s.Size, caption.MinSize, caption.MaxSize, false)
		h.colorPicker(c.Key("ink"), tr(c, "caption.textColor"), &s.TextColor, false)
		label := tr(c, "caption.alignment")
		strong(c, label).FontSize(12)
		index := 1
		if s.Alignment == caption.Left {
			index = 0
		} else if s.Alignment == caption.Right {
			index = 2
		}
		action.Segmented(c, &index, []string{tr(c, "caption.left"), tr(c, "caption.center"), tr(c, "caption.right")},
			action.SegmentedProps{Size: theme.Small, Block: true}).Label(label)
		s.Alignment = [...]caption.Alignment{caption.Left, caption.Center, caption.Right}[index]
	})
}
