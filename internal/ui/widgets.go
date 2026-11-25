package ui

import (
	"fmt"
	"math"

	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func choose[T any](b bool, a, z T) T {
	if b {
		return a
	}
	return z
}

func strong(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontWeight(600).TextColor(theme.TokensOf(c).Text).SingleLine()
}
func muted(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(12).TextColor(theme.TokensOf(c).TextSecondary).SingleLine()
}
func labelWrap(c *ui.Context, s string) {
	ui.Text(c, s).Wrap().TextColor(theme.TokensOf(c).Text)
}
func note(c *ui.Context, s string) {
	ui.Text(c, s).Wrap().FontSize(12).TextColor(theme.TokensOf(c).TextSecondary)
}
func mono(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).SingleLine().FontSize(12).FontFeatures("tnum").TextColor(theme.TokensOf(c).Text)
}
func icon(c *ui.Context, svg *ui.SVG, size float32, color ui.Color) {
	ui.Icon(c, svg).FontSize(size).TextColor(color).Shrink(0)
}
func button(c *ui.Context, s string, svg *ui.SVG, variant action.ButtonVariant, disabled bool) bool {
	return styledButton(c, s, svg, variant, disabled, false).Clicked()
}

func dangerButton(c *ui.Context, s string, svg *ui.SVG, disabled bool) bool {
	return styledButton(c, s, svg, action.Default, disabled, true).Clicked()
}

func styledButton(c *ui.Context, s string, svg *ui.SVG, variant action.ButtonVariant, disabled, danger bool) ui.Element {
	return styledButtonSize(c, s, svg, variant, theme.Middle, disabled, danger)
}

func styledButtonSize(c *ui.Context, s string, svg *ui.SVG, variant action.ButtonVariant, size theme.Size, disabled, danger bool) ui.Element {
	b := ui.ButtonBase(c).Disabled(disabled).Label(s).Shrink(0).AlignItems(ui.Center)
	action.StyleButton(c, b, "", action.ButtonProps{Variant: variant, Size: size})
	tk := theme.TokensOf(c)
	fg := tk.Text
	if variant == action.Primary {
		fg = tk.BgLayout
	}
	if danger && !disabled {
		fg = tk.Error
		b.TextColor(fg).Border(1, tk.ErrorBorder).Background(choose(b.Hovered(), tk.ErrorBg, tk.BgContainer))
	}
	if disabled {
		b.Background(tk.BgContainerDisabled).Border(1, tk.BorderDisabled).TextColor(tk.TextDisabled)
		fg = tk.TextDisabled
	}
	b.Children(func() {
		if svg != nil {
			icon(c, svg, 15, fg)
		}
		ui.Text(c, s).SingleLine()
	})
	return b
}

type iconSegment struct {
	label string
	icon  *ui.SVG
}

// iconSegments gives compact, keyboard-focusable icon toggles an accessible name.
func iconSegments(c *ui.Context, selected int, items []iconSegment) int {
	tk := theme.TokensOf(c)
	ui.Row(c).Gap(2).Padding(2).Shrink(0).Radius(tk.BorderRadius).Background(tk.FillQuaternary).Children(func() {
		for i, item := range items {
			b := ui.ButtonBase(c.Key(i)).Size(29, 25).Center().Radius(tk.BorderRadiusSM).Label(item.label).Tooltip(item.label)
			if i == selected {
				b.Background(tk.BgContainer).Border(1, tk.BorderSecondary)
			} else if b.Hovered() {
				b.Background(tk.FillSecondary)
			}
			b.Children(func() { icon(c, item.icon, 15, choose(i == selected, tk.Text, tk.TextSecondary)) })
			if b.Clicked() {
				selected = i
			}
		}
	})
	return selected
}
func smallIcon(c *ui.Context, svg *ui.SVG, title string, active, disabled bool) bool {
	return action.ActionIcon(c, svg, action.ActionIconProps{Title: title, Size: theme.Small, Active: active}).Disabled(disabled).Clicked()
}
func line(c *ui.Context) {
	ui.Box(c).Height(1).Shrink(0).AlignSelf(ui.Stretch).Background(theme.TokensOf(c).BorderSecondary)
}
func verticalLine(c *ui.Context) {
	ui.Box(c).Width(1).Shrink(0).AlignSelf(ui.Stretch).Background(theme.TokensOf(c).BorderSecondary)
}
func contentScroll(c *ui.Context, width, padding float32, body func()) {
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		ui.Column(c).WidthPercent(100).MaxWidth(width).AlignSelf(ui.Center).Padding(padding).Gap(12).Children(body)
	}).Grow(1).Basis(0).MinHeight(0).AlignSelf(ui.Stretch)
}

func compactInspector(c *ui.Context, title string, tools, body func()) {
	compactInspectorPadding(c, 20, title, tools, body)
}

func compactInspectorPadding(c *ui.Context, pad float32, title string, tools, body func()) {
	if title != "" || tools != nil {
		ui.Row(c).Height(57).Shrink(0).PaddingX(pad).Gap(8).AlignItems(ui.Center).Children(func() {
			strong(c, title).FontSize(15).MinWidth(0).Shrink(1)
			ui.Box(c).Grow(1)
			if tools != nil {
				tools()
			}
		})
	}
	top := float32(0)
	if title == "" && tools == nil {
		top = 16
	}
	layout.ScrollArea(c.Key("inspector-scroll"), layout.ScrollAreaProps{}, func() {
		ui.Column(c).Padding(top, pad, 20, pad).Gap(12).Children(body)
	}).Grow(1).Basis(0).MinHeight(0).AlignSelf(ui.Stretch)
}

func detailRow(c *ui.Context, label, value string) {
	ui.Row(c).MinHeight(29).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, label).Shrink(0)
		ui.Box(c).Grow(1)
		mono(c, value).Shrink(1).MinWidth(0).Tooltip(value)
	})
}

func radioOption[T comparable](c *ui.Context, val *T, v T, label, desc string, disabled bool) {
	tk := theme.TokensOf(c)
	ui.Column(c).Padding(7, 9).Gap(3).Radius(tk.BorderRadius).
		Background(choose(*val == v, tk.FillQuaternary, tk.BgContainer)).Children(func() {
		choice.Radio(c, val, v, label, choice.RadioProps{Disabled: disabled}).AlignSelf(ui.Stretch)
		if desc != "" {
			ui.Column(c).Padding(0, 0, 0, 23).Children(func() { note(c, desc) })
		}
	})
}
func pathLine(c *ui.Context, path string, svg *ui.SVG) {
	tk := theme.TokensOf(c)
	if path == "" {
		path = tr(c, "common.noneSelected")
	}
	ui.Row(c).MinHeight(37).Padding(8, 10).Gap(8).Radius(tk.BorderRadius).Background(tk.FillQuaternary).AlignItems(ui.Start).Tooltip(path).Children(func() {
		ui.Box(c).Height(20).Center().Children(func() { icon(c, svg, 15, tk.TextSecondary) })
		ui.Text(c, path).Wrap().MaxLines(3).MinWidth(0).Grow(1).FontSize(12).TextColor(tk.TextSecondary)
	})
}
func switchRow(c *ui.Context, title string, svg *ui.SVG, value *bool, disabled bool) {
	ui.Row(c).Height(30).Gap(8).AlignItems(ui.Center).Children(func() {
		if svg != nil {
			icon(c, svg, 15, theme.TokensOf(c).TextSecondary)
		}
		ui.Text(c, title).FontSize(13).SingleLine()
		ui.Box(c).Grow(1)
		choice.Switch(c, value, choice.SwitchProps{}).Label(title).Disabled(disabled)
	})
}
func two(c *ui.Context, a, b func()) {
	ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(6).Children(a)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(6).Children(b)
	})
}
func input(c *ui.Context, label, placeholder string, v *string, disabled bool) ui.Element {
	var edit ui.Element
	ui.Column(c).Gap(5).Children(func() {
		strong(c, label).FontSize(12)
		edit = field.Input(c, v, field.InputProps{Placeholder: placeholder}).Label(label).Disabled(disabled).AlignSelf(ui.Stretch)
	})
	return edit
}
func number(c *ui.Context, label string, v *int, lo, hi int, disabled bool) {
	ui.Column(c).Gap(5).Children(func() {
		strong(c, label).FontSize(12)
		numberField(c, label, v, lo, hi, disabled)
	})
}
func numberField(c *ui.Context, label string, v *int, lo, hi int, disabled bool) {
	f := float64(*v)
	field.InputNumber(c, &f, field.InputNumberProps{Min: float64(lo), Max: float64(hi)}).Label(label).Disabled(disabled).AlignSelf(ui.Stretch)
	if !math.IsNaN(f) && !math.IsInf(f, 0) {
		*v = int(math.Round(f))
	}
}
func (h *historyControls) colorPicker(c *ui.Context, label string, v *string, disabled bool) {
	r, g, b, err := imaging.ParseColor(*v)
	if err != nil {
		r, g, b = 255, 255, 255
	}
	old := ui.RGB(r, g, b)
	value := old
	ui.Column(c).Gap(5).Children(func() {
		strong(c, label).FontSize(12)
		e := choice.ColorPicker(c, &value, choice.ColorPickerProps{Trigger: choice.ColorTriggerField, Disabled: disabled,
			Presets: []ui.Color{ui.RGB(255, 255, 255), ui.RGB(241, 241, 241), ui.RGB(34, 34, 34), ui.RGB(0, 0, 0)}}).Label(label).AlignSelf(ui.Stretch)
		h.trackColor(e, value != old)
	})
	if value != old {
		*v = fmt.Sprintf("#%02X%02X%02X", value.R, value.G, value.B)
	}
}
func (h *historyControls) slider(c *ui.Context, key string, before, v *int, lo, hi int, disabled bool) {
	label := tr(c, key)
	mixed := h.hasMixed(key)
	ui.Column(c).Gap(4).Children(func() {
		strong(c, label).FontSize(12)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			f := float64(*v)
			s := choice.Slider(c, &f, choice.SliderProps{Min: float64(lo), Max: float64(hi), Step: 1, Disabled: disabled}).Grow(1).MinWidth(0).Label(label)
			pressed, held := s.PressedButton()
			changed := int(math.Round(f)) != *v || mixed && s.Pressed()
			h.track(s, held && pressed == 0, changed)
			*v = int(math.Round(f))
			ui.Column(c).Width(84).Children(func() {
				if mixed {
					if mixedNumberField(c.Key(h.model.ScopeRevision), label, v, lo, hi, disabled) || changed {
						*before = lo - 1
					}
				} else {
					numberField(c, label, v, lo, hi, disabled)
				}
			})
		})
	})
}
func empty(c *ui.Context, title, desc string, svg *ui.SVG) {
	ui.Column(c).Grow(1).MinHeight(100).Center().Children(func() {
		display.Empty(c, display.EmptyProps{Title: title, Description: desc, Icon: svg})
	})
}
func sizeText(w, h int) string {
	if w == 0 || h == 0 {
		return "—"
	}
	return fmt.Sprintf("%d × %d px", w, h)
}
func byteText(c *ui.Context, n int64) string {
	key, size, digits := "size.bytes", float64(n), 0
	switch {
	case n >= 1<<20:
		key, size, digits = "size.megabytes", size/(1<<20), 2
	case n >= 1<<10:
		key, size, digits = "size.kilobytes", size/(1<<10), 1
	}
	text := i18n.FormatNumber(c.Locale(), size, i18n.NumberOptions{
		NoGrouping: true, Integer: digits == 0, MinFractionDigits: digits, MaxFractionDigits: digits,
	})
	return trf(c, key, l10n.Args{"size": text})
}
