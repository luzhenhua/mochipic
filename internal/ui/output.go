package ui

import (
	"fmt"
	"strings"

	"imageflow/internal/files"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

const namePerPage = 12

func (d *desktop) copyPlan(c *ui.Context) {
	m := d.model
	if m.Planning || m.PlanErr != nil {
		return
	}
	var rows strings.Builder
	for _, e := range m.Plan.Entries {
		value := e.Relative
		switch {
		case e.Issue != nil:
			value = trf(c, "output.planErrorDetails", l10n.Args{"reason": errText(c, e.Issue)})
		case e.Skip:
			value = trf(c, "output.skippedName", l10n.Args{"name": value})
		}
		fmt.Fprintf(&rows, "%s\t%s\n", sourceName(c, e.Source), value)
	}
	mygo.Clipboard.WriteText(rows.String())
}

func (d *desktop) planPage(c *ui.Context, export bool) {
	m, tk := d.model, theme.TokensOf(c)
	if len(m.Sources) == 0 {
		empty(c, tr(c, "output.empty"), tr(c, "output.emptyHint"), lucide.FileImage())
		return
	}
	if m.Planning && len(m.Plan.Entries) == 0 && m.PlanErr == nil {
		ui.Box(c).Grow(1).Center().Children(func() { feedback.Spin(c, feedback.SpinProps{Tip: tr(c, "output.planning")}, nil) })
		return
	}
	if m.PlanErr != nil {
		ui.Column(c).Padding(24).Children(func() {
			feedback.Alert(c, tr(c, "output.planFailed"), feedback.AlertProps{Type: feedback.Error, Description: errText(c, m.PlanErr)})
		})
		return
	}
	entries := m.Plan.Entries
	if export {
		d.outputTreeView(c, entries)
		return
	}
	start, end := pageRange(len(entries), namePerPage, &d.namePage)
	paths := make([]string, 0, end-start)
	for _, e := range entries[start:end] {
		paths = append(paths, e.Source.Path)
	}
	m.ShowThumbnails(paths)
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).Padding(18, 24, 0, 24).Children(func() {
		d.scopePicker(c.Key("rename-picker"))
		ui.Row(c).Height(38).Shrink(0).PaddingX(12).Gap(10).AlignItems(ui.Center).Radius(tk.BorderRadiusSM).Background(tk.BgLayout).Children(func() {
			muted(c, tr(c, "rename.index")).Width(50)
			muted(c, tr(c, "rename.sourceName")).Grow(1).Basis(0).MinWidth(0)
			ui.Box(c).Width(22).Shrink(0)
			muted(c, tr(c, "rename.outputName")).Grow(1).Basis(0).MinWidth(0)
		})
		layout.ScrollArea(c.Key(d.namePage), layout.ScrollAreaProps{}, func() {
			ui.Column(c).Children(func() {
				for _, e := range entries[start:end] {
					d.planRow(c.Key(e.Source.ID()), e)
				}
			})
		}).Grow(1).Basis(0).MinHeight(0).AlignSelf(ui.Stretch)
	})
}

func (d *desktop) planRow(c *ui.Context, e files.Entry) {
	tk := theme.TokensOf(c)
	path := e.Source.Path
	name := sourceName(c, e.Source)
	row := ui.ButtonBase(c).Height(56).PaddingX(12).Gap(10).AlignItems(ui.Center).Radius(tk.BorderRadiusSM).Label(name)
	row.AccessSelected(d.model.EditSelection[path])
	if d.model.EditSelection[path] || row.Hovered() {
		row.Background(tk.FillQuaternary)
	}
	row.Children(func() {
		muted(c, fmt.Sprintf("%03d", e.Source.Ordinal+1)).Width(50)
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(9).AlignItems(ui.Center).Children(func() {
			d.thumb(c.Key("rename-thumb"), path, 48, 37)
			strong(c, name).MinWidth(0).Shrink(1).Tooltip(path)
		})
		ui.Box(c).Width(22).Center().Children(func() { icon(c, lucide.ArrowRight(), 14, tk.TextTertiary) })
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
			value, full := baseName(c, e.Relative), e.Relative
			if e.Issue != nil {
				value, full = tr(c, "rename.invalidName"), errText(c, e.Issue)
			}
			mono(c, value).Grow(1).Basis(0).MinWidth(0).Tooltip(full)
			entryTag(c, e.Issue, e.Skip, e.Requested != e.Relative)
		})
	})
	if row.Clicked() {
		d.pickThumbnail(path, row.ClickModifiers())
		if d.model.Selected == path {
			d.model.SetPreviewPart(e.Source.Part)
		}
	}
	if row.Shortcut(ui.Cmd, ui.KeyA) && !c.FocusTakesText() {
		d.model.SelectAll()
		c.Invalidate()
	}
	line(c)
}

func (d *desktop) outputTreeView(c *ui.Context, entries []files.Entry) {
	const per = 18
	rows := d.treeRows(entries)
	start, end := pageRange(len(rows), per, &d.treePage)
	var visible []string
	for _, r := range rows[start:end] {
		if r.entry != nil {
			visible = append(visible, r.entry.Source.ID())
		}
	}
	d.model.ShowThumbnails(visible)
	contentScroll(c.Key("output-tree"), 1000, 24, func() {
		ui.Row(c).Height(30).Gap(6).AlignItems(ui.Center).Children(func() {
			strong(c, tr(c, "output.treePreview")).FontSize(16)
			ui.Box(c).Grow(1)
			if len(rows) > per {
				pager(c, len(rows), per, &d.treePage)
			}
			if smallIcon(c, lucide.RefreshCw(), tr(c, "output.refreshPlan"), false, false) {
				d.model.RefreshPlan()
			}
		})
		ui.Column(c).Children(func() {
			for i, r := range rows[start:end] {
				if (i == 0 && r.entry != nil) || (i == 1 && start == 0 && rows[0].folder != nil) {
					line(c)
					ui.Row(c).Gap(8).Height(23).PaddingX(10).Children(func() {
						muted(c, tr(c, "rename.outputName")).Grow(1).Basis(0)
						muted(c, tr(c, "scope.edits")).Grow(1).Basis(0)
						muted(c, tr(c, "resize.outputSize"))
					})
					line(c)
				}
				key := "folder:"
				if r.folder != nil {
					key += r.folder.path
				} else {
					key = r.entry.Source.ID()
				}
				d.drawTreeRow(c.Key(key), r)
				if r.entry != nil {
					line(c)
				}
			}
		})
	})
}

func (d *desktop) renameInspector(c *ui.Context) {
	m := d.model
	r := m.Naming
	before := r
	h := d.editControls()
	defer h.finish()
	edit := func(label, placeholder string, value *string) {
		old := *value
		field := input(c, label, placeholder, value, false)
		h.track(field, field.Focused(), old != *value)
	}
	compactInspector(c, tr(c, "rename.rules"), nil, func() {
		ui.Column(c).Gap(5).Children(func() {
			strong(c, tr(c, "common.mode")).FontSize(12)
			labels := []string{tr(c, "rename.keepOriginal"), tr(c, "rename.templateMode")}
			if d.hasMixed("common.mode") {
				if i, ok := mixedChoice(c, tr(c, "common.mode"), labels); ok {
					r.KeepOriginal = i == 0
					before.KeepOriginal = !r.KeepOriginal
				}
			} else {
				v := choose(r.KeepOriginal, 0, 1)
				action.Segmented(c, &v, labels, action.SegmentedProps{Block: true, Size: theme.Small})
				r.KeepOriginal = v == 0
			}
		})
		if r.KeepOriginal {
			return
		}
		two(c, func() { edit(tr(c, "rename.prefix"), tr(c, "common.none"), &r.Prefix) }, func() { edit(tr(c, "rename.suffix"), tr(c, "common.none"), &r.Suffix) })
		switchRow(c, tr(c, "rename.addSequence"), nil, &r.Sequence, false)
		two(c, func() { number(c, tr(c, "rename.startIndex"), &r.Start, 0, 999_999_999, !r.Sequence) }, func() { number(c, tr(c, "rename.digits"), &r.Digits, 1, 9, !r.Sequence) })
		line(c)
		text, hint := r.Pattern(), "{original}"
		if d.hasMixed("rename.pattern") {
			text, hint = "", tr(c, "scope.mixed")
		}
		original := text
		edit(tr(c, "rename.pattern"), hint, &text)
		if text != original {
			r.Template = text
			before.Template = original
		}
		ui.Column(c).Gap(6).Children(func() {
			strong(c, tr(c, "rename.variables")).FontSize(12)
			ui.Row(c).Gap(5).Wrap().Children(func() {
				for _, name := range []string{"{original}", "{prefix}", "{suffix}", "{index}", "{date}"} {
					btn := action.Button(c.Key(name), name, action.ButtonProps{Variant: action.Filled, Size: theme.Small}).
						Label(trf(c, "rename.insertVariable", l10n.Args{"variable": name})).Tooltip(trf(c, "rename.insertVariable", l10n.Args{"variable": name})).Disabled(name == "{index}" && !r.Sequence)
					if btn.Clicked() {
						r.Template = r.Pattern() + name
					}
				}
			})
		})
		if r.Template != "" && button(c, tr(c, "rename.resetTemplate"), lucide.RotateCcw(), action.Text, false) {
			r.Template = ""
		}
	})
	if r != before {
		m.EditNamingFrom(before, r, h.group)
		c.Invalidate()
	}
}

func (d *desktop) outputInspector(c *ui.Context) {
	m := d.model
	o := m.Output
	compactInspector(c, tr(c, "output.folder"), nil, func() {
		tk := theme.TokensOf(c)
		name := baseName(c, o.Directory)
		folder := ui.ButtonBase(c.Key("destination")).Height(35).PaddingX(10).Gap(8).AlignItems(ui.Center).Radius(tk.BorderRadius).Border(1, tk.Border).Label(tr(c, "output.chooseFolder")).Tooltip(o.Directory)
		folder.Children(func() {
			icon(c, lucide.FolderOpen(), 15, tk.TextSecondary)
			strong(c, name).FontSize(13).Grow(1).MinWidth(0)
			icon(c, lucide.ChevronDown(), 13, tk.TextTertiary)
		})
		if folder.Clicked() {
			d.pickDirectory(false)
		}
		ui.Column(c).Gap(7).Children(func() {
			strong(c, tr(c, "output.conflict")).FontSize(12)
			v := choose(o.Conflict == files.Skip, 1, 0)
			action.Segmented(c, &v, []string{tr(c, "output.autoRename"), tr(c, "output.skipExisting")}, action.SegmentedProps{Block: true, Size: theme.Small})
			o.Conflict = choose(v == 1, files.Skip, files.Rename)
		})
		choice.Checkbox(c, &o.KeepTree, tr(c, "output.keepTree"), choice.CheckboxProps{})
	})
	if o != m.Output {
		m.SetOutput(o)
		c.Invalidate()
	}
}
