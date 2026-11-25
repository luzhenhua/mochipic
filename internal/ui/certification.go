package ui

import (
	"image"

	"imageflow/internal/app"
	"imageflow/internal/certification"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (d *desktop) certificationInspector(c *ui.Context) {
	m := d.model
	group := m.Certification()
	before := group
	h := d.editControls()
	defer h.finish()
	compactInspectorPadding(c, 16, tr(c, "certification.builtIn"), func() {
		if styledButtonSize(c, tr(c, "certification.import"), lucide.Plus(), action.Text, theme.Small, !m.CanImportCertification(), false).
			Tooltip(tr(c, "certification.choosePNG")).Clicked() {
			d.pick(mygo.OpenDialogOptions{
				Title: d.t("certification.choosePNG"), ButtonLabel: d.t("common.use"),
				Filters: []mygo.FileFilter{{Name: "PNG", Extensions: []string{"png"}}},
			}, func(paths []string) { m.ImportCertification(paths[0]) })
		}
	}, func() {
		builtins := certification.Builtins()
		d.badgeGrid(c.Key("built-in-badges"), builtins[:], &group)
		if m.CertificationError != nil {
			note(c, errText(c, m.CertificationError))
		}
		if m.CertificationImporting {
			note(c, tr(c, "certification.importing"))
		}
		var custom []*certification.Badge
		for _, b := range m.CertificationBadges() {
			if b.LocalPath() != "" {
				custom = append(custom, b)
			}
		}
		if len(custom) != 0 {
			strong(c, tr(c, "certification.custom")).FontSize(12)
			d.badgeGrid(c.Key("custom-badges"), custom, &group)
		}
		line(c)
		before = group
		h.slider(c, "certification.size", &before.Size, &group.Size, certification.MinSize, certification.MaxSize, false)
		h.slider(c, "certification.gap", &before.Gap, &group.Gap, 0, certification.MaxGap, false)
		anchorControls(c, &group.Placement, tr(c, "certification.dragHint"))
	})
	if group != before {
		m.EditCertificationFrom(before, group, h.group)
		c.Invalidate()
	}
}

func (d *desktop) badgeGrid(c *ui.Context, badges []*certification.Badge, group *certification.Group) {
	const columns = 5
	tk, m := theme.TokensOf(c), d.model
	ui.Column(c).Gap(6).Children(func() {
		for start := 0; start < len(badges); start += columns {
			ui.Row(c.Key(start)).Gap(6).Children(func() {
				end := min(start+columns, len(badges))
				for _, b := range badges[start:end] {
					preview := m.CertificationPreview(b)
					selected := group.Has(b)
					mixed := false
					if len(m.EditSelection) > 1 {
						n := d.mixed.badges[b]
						selected = n == len(m.EditSelection)
						mixed = n > 0 && !selected
					}
					tile := ui.ButtonBase(c.Key(b)).Column().Grow(1).Basis(0).MinWidth(0).Height(66).
						Gap(5).Center().Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).
						Role(ui.RoleToggleButton).Label(b.Name()).Tooltip(b.Name()).Checked(selected).Disabled(preview.Pixels == nil)
					fill, edge := tk.FillQuaternary, tk.BorderSecondary
					switch {
					case selected:
						fill, edge = tk.FillSecondary, tk.TextTertiary
					case tile.Hovered():
						fill, edge = tk.FillTertiary, tk.Border
					}
					tile.Background(fill).Border(1, edge).Children(func() {
						if preview.Pixels != nil {
							display.Image(c, bitmap(tile, preview.Pixels), display.ImageProps{
								Size: 36, Contain: true, NoPreview: true, Variant: display.ImageBorderless,
							}).Radius(0).IgnoreInput(true)
						} else {
							ui.Box(c).Size(36, 36).Radius(18).Background(tk.FillTertiary)
						}
						ui.Text(c, b.Name()).FontSize(10).FontWeight(choose(selected, 600, 500)).
							TextColor(choose(selected, tk.Text, tk.TextSecondary)).SingleLine().MaxWidthPercent(100)
						if selected || mixed {
							check := ui.Box(c).Absolute().Right(3).Top(3).Size(13, 13).Radius(7).
								Center().Background(tk.Primary).Border(1, tk.BgContainer).IgnoreInput(true)
							check.Children(func() { icon(c, choose(mixed, lucide.Minus(), lucide.Check()), 8, tk.BgContainer) })
						}
					})
					if tile.Clicked() {
						m.SetBadge(b, !selected)
						*group = m.Certification()
						c.Invalidate()
					}
				}
				for range columns - (end - start) {
					ui.Box(c).Grow(1).Basis(0).MinWidth(0)
				}
			})
		}
	})
}

func (d *desktop) certificationHandle(c *ui.Context, frame app.PreviewFrame, w, h float32) {
	m := d.model
	if frame.CertificationBounds.Empty() || !m.OriginalReady() {
		return
	}
	size := image.Pt(frame.Info.Width, frame.Info.Height)
	key := struct {
		path string
		part int
	}{m.Selected, m.PreviewPart}
	markHandle(c.Key(key), m, frame.CertificationBounds, size, w, h, len(frame.Layers)+1, tr(c, "certification.dragLabel"), false, nil, func(pos image.Point, group app.HistoryGroup) {
		g := m.Certification()
		g.MoveTo(pos, size, frame.CertificationBounds.Size())
		if g != m.Certification() {
			m.EditCertification(g, group)
			c.Invalidate()
		}
	})
}
