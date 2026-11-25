package ui

import (
	"slices"
	"strconv"
	"strings"

	"imageflow/internal/certification"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (d *desktop) editPage() bool {
	switch d.page {
	case watermark, resize, captionPage, certificationPage, format, rename:
		return true
	}
	return false
}
func (d *desktop) scopeHeader(c *ui.Context) {
	m := d.model
	n := len(m.EditSelection)
	key := "scope.current"
	title := "scope.edit"
	svg := lucide.Image()
	if d.page == output {
		title, key, n, svg = "scope.export", "scope.all", len(m.Sources), lucide.FolderOutput()
	} else if n > 1 {
		key = choose(n == len(m.Sources), "scope.all", "scope.selected")
		svg = lucide.Images()
	}
	ui.Row(c.Key("scope-label")).Height(44).Shrink(0).PaddingX(18).Gap(6).AlignItems(ui.Center).Children(func() {
		muted(c, tr(c, title))
		ui.Box(c).Grow(1)
		icon(c, svg, 14, theme.TokensOf(c).TextSecondary)
		strong(c, trf(c, key, l10n.Args{"count": n})).FontSize(12)
	})
	line(c)
	if d.editPage() {
		d.mixedSummary(c)
	}
}
func (d *desktop) scopePicker(c *ui.Context) {
	m := d.model
	ui.Row(c).Height(29).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		strong(c, trf(c, "scope.batchCount", l10n.Args{"count": len(m.Sources)})).FontSize(12)
		ui.Box(c).Grow(1)
		if len(m.Sources) < 2 {
			return
		}
		d.filmstripHint(c)
		all := len(m.EditSelection) == len(m.Sources)
		label := tr(c, choose(all, "scope.onlyCurrent", "scope.selectAll"))
		if styledButtonSize(c.Key("select"), label, nil, action.Text, theme.Small, false, false).Tooltip(choose(all, label, shortcutLabel("A"))).Clicked() {
			if all {
				m.Select(m.Selected)
			} else {
				m.SelectAll()
			}
			c.Invalidate()
		}
	})
}

type mixedInspector struct {
	revision uint64
	page     page
	tab      int
	logo     string
	fields   []string
	badges   map[*certification.Badge]int
}

func (d *desktop) mixedFields() []string {
	m := d.model
	tab := 0
	if d.page == watermark {
		tab = d.watermarkTab
	} else if d.page == resize {
		tab = d.resizeTab
	}
	if len(m.EditSelection) < 2 {
		return nil
	}
	if x := &d.mixed; x.revision == m.ScopeRevision && x.page == d.page && x.tab == tab && x.logo == d.logoSelection {
		return x.fields
	}
	state := mixedInspector{revision: m.ScopeRevision, page: d.page, tab: tab, logo: d.logoSelection}
	add := func(key string, different bool) {
		if different && !slices.Contains(state.fields, key) {
			state.fields = append(state.fields, key)
		}
	}
	placement := func(a, b imaging.Placement) {
		add("common.location", a.Free != b.Free || a.Anchor != b.Anchor)
		add("watermark.marginX", a.MarginX != b.MarginX)
		add("watermark.marginY", a.MarginY != b.MarginY)
	}
	ref := m.SelectedSource()
	o := m.Image
	seen := make(map[*files.PhotoSettings]bool)
	for _, s := range m.Sources {
		if s.Path == ref.Path || !m.EditSelection[s.Path] {
			continue
		}
		switch d.page {
		case captionPage:
			a, b := m.CaptionFor(ref), m.CaptionFor(s)
			add("caption.title", a.Text != b.Text)
			add("caption.opacity", a.Style.Opacity != b.Style.Opacity)
			add("caption.background", a.Style.Background != b.Style.Background)
			add("caption.height", a.Style.Height != b.Style.Height)
			add("caption.alignment", a.Style.Alignment != b.Style.Alignment)
			add("caption.size", a.Style.Size != b.Style.Size)
			add("caption.textColor", a.Style.TextColor != b.Style.TextColor)
			continue
		case certificationPage:
			if state.badges == nil {
				state.badges = make(map[*certification.Badge]int)
				for i := 0; i < ref.Certification.Count(); i++ {
					state.badges[ref.Certification.Badge(i)]++
				}
			}
			for i := 0; i < s.Certification.Count(); i++ {
				state.badges[s.Certification.Badge(i)]++
			}
			a, b := ref.Certification, s.Certification
			if a == (certification.Group{}) {
				a = certification.Defaults()
			}
			if b == (certification.Group{}) {
				b = certification.Defaults()
			}
			add("certification.size", a.Size != b.Size)
			add("certification.gap", a.Gap != b.Gap)
			placement(a.Placement, b.Placement)
			for i := 0; i < max(a.Count(), b.Count()); i++ {
				if a.Badge(i) != b.Badge(i) {
					add("certification.builtIn", true)
					break
				}
			}
			continue
		case resize:
			if tab == 0 {
				add("crop.tab", ref.Crop != s.Crop)
				continue
			}
		}
		if s.Settings == ref.Settings || seen[s.Settings] {
			continue
		}
		seen[s.Settings] = true
		photo := m.PhotoSettings(s)
		other := photo.Image
		switch d.page {
		case watermark:
			switch tab {
			case 0:
				a, b := o.Text, other.Text
				add("watermark.textContent", a.Text != b.Text)
				add("watermark.font", a.Font != b.Font)
				add("watermark.fontSize", a.Size != b.Size)
				add("watermark.color", a.Color != b.Color)
				add("watermark.opacity", a.Opacity != b.Opacity)
				add("watermark.scale", a.Scale != b.Scale)
				add("watermark.stroke", a.Stroke != b.Stroke)
				add("watermark.shadow", a.Shadow != b.Shadow)
				placement(a.Placement, b.Placement)
			default:
				a, b := o.LogoMarks(), other.LogoMarks()
				add("watermark.layers", len(a) != len(b) || o.TextLayerPosition(len(a)) != other.TextLayerPosition(len(b)))
				for i := 0; i < min(len(a), len(b)); i++ {
					add("watermark.layers", a[i].Path != b[i].Path)
				}
				l, r := o.LogoByPath(d.logoSelection), other.LogoByPath(d.logoSelection)
				if len(a) == 0 && len(b) == 0 {
					l, r = &o.Logo, &other.Logo
				}
				if l != nil && r != nil {
					add("watermark.widthPercent", l.Width != r.Width)
					add("watermark.heightPercent", l.Height != r.Height)
					add("watermark.opacity", l.Opacity != r.Opacity)
					add("watermark.keepRatio", l.KeepRatio != r.KeepRatio)
					placement(l.Placement, r.Placement)
				}
			}
		case resize:
			a, b := o.Resize, other.Resize
			add("resize.enable", a.Enabled != b.Enabled)
			add("common.mode", a.Mode != b.Mode)
			add("resize.width", a.Width != b.Width)
			add("resize.height", a.Height != b.Height)
			add("resize.scale", a.Percent != b.Percent)
			add("resize.longEdgeMax", a.Limit != b.Limit)
			add("resize.fitMode", a.Fit != b.Fit)
			add("resize.lockRatio", a.LockRatio != b.LockRatio)
			add("resize.paddingColor", a.PadColor != b.PadColor)
			add("resize.transparentPadding", a.Transparent != b.Transparent)
		case format:
			add("format.outputFormat", o.Format != other.Format)
			add("format.quality", o.Quality != other.Quality)
			add("format.webpEncoding", o.Lossless != other.Lossless)
			add("format.transparency", o.FlattenAlpha != other.FlattenAlpha)
			add("format.backgroundColor", o.Background != other.Background)
		case rename:
			a, b := m.Naming, photo.Naming
			add("common.mode", a.KeepOriginal != b.KeepOriginal)
			add("rename.pattern", a.Pattern() != b.Pattern())
			add("rename.prefix", a.Prefix != b.Prefix)
			add("rename.suffix", a.Suffix != b.Suffix)
			add("rename.addSequence", a.Sequence != b.Sequence)
			add("rename.startIndex", a.Start != b.Start)
			add("rename.digits", a.Digits != b.Digits)
		}
	}
	d.mixed = state
	return state.fields
}
func (d *desktop) hasMixed(key string) bool {
	if key == "caption.heightPercent" {
		key = "caption.height"
	}
	return slices.Contains(d.mixedFields(), key)
}
func (d *desktop) mixedSummary(c *ui.Context) {
	keys := d.mixedFields()
	var labels []string
	for _, k := range keys {
		if d.page == watermark && d.watermarkTab == 0 && (k == "watermark.fontSize" || k == "watermark.textContent" || k == "watermark.font") || d.page == captionPage && k == "caption.title" {
			continue
		}
		labels = append(labels, tr(c, k))
	}
	if len(labels) == 0 {
		return
	}
	text := tr(c, "scope.mixed") + " · " + strings.Join(labels, choose(c.Locale() == "zh-Hans" || c.Locale() == "ja", "、", ", "))
	muted(c, text).Margin(5, 18, 0, 18).MinWidth(0).MaxLines(1).Tooltip(text)
}

// The native number spinner always formats a number. Keep the mixed draft
// empty until Enter or blur, then apply even a value matching the preview.
func mixedNumberField(c *ui.Context, label string, v *int, lo, hi int, disabled bool) bool {
	type draft struct {
		text    string
		focused bool
	}
	var changed bool
	box := ui.Column(c).AlignSelf(ui.Stretch)
	st := ui.Local(box, "mixed-number", func() draft { return draft{} })
	box.Children(func() {
		edit := field.Input(c, &st.text, field.InputProps{Placeholder: tr(c, "scope.mixed")}).Label(label).Disabled(disabled).AlignSelf(ui.Stretch)
		if edit.Submitted() || st.focused && !edit.Focused() {
			if n, err := strconv.Atoi(strings.TrimSpace(st.text)); err == nil {
				*v = max(lo, min(hi, n))
				changed = true
			}
			st.text = ""
		}
		st.focused = edit.Focused()
	})
	return changed
}

// A mixed segmented field starts with no chosen option. Lobe Segmented always
// selects its first segment, so use the native Lobe Select placeholder here.
func mixedChoice(c *ui.Context, label string, labels []string) (int, bool) {
	value := ""
	options := make([]field.SelectOption, len(labels))
	for i, name := range labels {
		options[i] = field.SelectOption{Value: strconv.Itoa(i), Label: name}
	}
	field.Select(c, &value, field.SelectProps{Options: options, Placeholder: tr(c, "scope.mixed"), Size: theme.Small}).Label(label).AlignSelf(ui.Stretch)
	n, err := strconv.Atoi(value)
	return n, err == nil
}

func (d *desktop) scopeNumber(c *ui.Context, key string, before, value *int, lo, hi int, disabled bool) {
	label := tr(c, key)
	if d.hasMixed(key) {
		ui.Column(c).Gap(5).Children(func() {
			strong(c, label).FontSize(12)
			if mixedNumberField(c.Key(d.model.ScopeRevision), label, value, lo, hi, disabled) {
				*before = lo - 1
			}
		})
		return
	}
	number(c, label, value, lo, hi, disabled)
}
