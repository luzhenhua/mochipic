package ui

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"imageflow/internal/batch"
	"imageflow/internal/config"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

const maxPresets = 64

func (d *desktop) presetsPage(c *ui.Context) {
	m := d.model
	items := m.Preferences.Presets
	if _, ok := m.Preferences.Preset(d.chosenPreset); !ok && len(items) > 0 {
		d.chosenPreset = items[0].ID
	}
	if len(items) == 0 {
		empty(c, tr(c, "preset.empty"), tr(c, "preset.emptyHint"), lucide.Layers())
	} else {
		contentScroll(c.Key("presets-list"), 868, 24, func() {
			ui.Column(c).Gap(3).Children(func() {
				for _, p := range items {
					d.presetRow(c.Key(p.ID), p)
				}
			})
		})
	}
	if feedback.Modal(c.Key("save-preset"), &d.presetModal, feedback.ModalProps{
		Title: choose(d.editingPreset == "", tr(c, "preset.create"), tr(c, "preset.edit")), OkText: tr(c, "common.save"), CancelText: tr(c, "common.cancel"), Width: 420,
	}, func() {
		ui.Column(c).Gap(9).Children(func() {
			input(c, tr(c, "preset.name"), tr(c, "preset.nameExample"), &d.presetName, false)
			if d.editingPreset == "" {
				note(c, tr(c, "preset.saveHint"))
			} else {
				switchRow(c, tr(c, "preset.replaceSettings"), nil, &d.presetReplace, false)
			}
		})
	}) {
		d.savePreset()
	}
}

func presetIcon(id string) *ui.SVG {
	switch id {
	case "builtin-web":
		return lucide.Image()
	case "builtin-detail":
		return lucide.PanelsTopLeft()
	case "builtin-social":
		return lucide.Square()
	default:
		return lucide.SlidersHorizontal()
	}
}

func (d *desktop) presetRow(c *ui.Context, p config.Preset) {
	m, tk := d.model, theme.TokensOf(c)
	active := d.chosenPreset == p.ID
	applied := p.ID == m.AppliedPreset && !m.SelectionHasMixedPresets()
	name := presetDisplayName(c, p)
	row := ui.Row(c).Height(74).Padding(0, 12, 0, 2).Gap(7).Radius(tk.BorderRadiusLG).AlignItems(ui.Center)
	row.Children(func() {
		btn := ui.ButtonBase(c).Height(74).Grow(1).Basis(0).MinWidth(0).Gap(12).PaddingX(10).AlignItems(ui.Center).Label(name)
		if active || btn.Hovered() {
			row.Background(tk.FillQuaternary)
		}
		btn.Children(func() {
			ui.Box(c).Size(46, 46).Shrink(0).Center().Radius(tk.BorderRadius).
				Background(choose(active, tk.BgContainer, tk.FillQuaternary)).Children(func() { icon(c, presetIcon(p.ID), 20, tk.TextSecondary) })
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
				strong(c, name).FontSize(15)
				muted(c, strings.Join([]string{resizeText(c, p.Image.Resize), formatText(c, p.Image), markText(c, p.Image)}, "  ·  "))
			})
			if p.ID == m.Preferences.DefaultPreset {
				tag(c, tr(c, "preset.default"), "")
			}
			if applied {
				icon(c, lucide.Check(), 15, tk.SuccessText)
			}
		})
		if btn.Clicked() {
			d.chosenPreset = p.ID
		}
		open := ui.Local(row, "actions", func() bool { return false })
		options := []overlay.MenuItem{
			{Key: "apply", Label: tr(c, "preset.apply"), Icon: lucide.Check(), Disabled: applied},
			{Key: "default", Label: tr(c, "preset.makeDefault"), Icon: lucide.Star(), Disabled: p.ID == m.Preferences.DefaultPreset},
			{Key: "edit", Label: tr(c, "preset.edit"), Icon: lucide.Pencil()},
		}
		if customPreset(p.ID) {
			options = append(options, overlay.MenuItem{Key: "delete", Label: tr(c, "preset.delete"), Icon: lucide.Trash2(), Danger: true})
		}
		switch overlay.DropdownMenu(c.Key("preset-actions"), open, overlay.DropdownMenuProps{Items: options}, func(b ui.Element) {
			action.StyleButton(c, b, "", action.ButtonProps{Variant: action.Text, Size: theme.Small})
			b.Size(26, 26).Center().Label(trf(c, "preset.actionsFor", l10n.Args{"name": name})).Tooltip(tr(c, "preset.actions"))
			b.Children(func() { icon(c, lucide.Ellipsis(), 17, tk.TextSecondary) })
		}) {
		case "apply":
			d.chosenPreset = p.ID
			m.ApplyPreset(p)
		case "default":
			d.chosenPreset = p.ID
			d.setDefaultPreset(p.ID)
		case "edit":
			d.chosenPreset = p.ID
			d.editPreset(p)
		case "delete":
			d.deletePreset(p.ID)
		}
	})
}

func customPreset(id string) bool { return !strings.HasPrefix(id, "builtin-") }

func (d *desktop) setDefaultPreset(id string) {
	p := d.model.Preferences
	p.DefaultPreset = id
	d.model.SetPreferences(p)
}

func (d *desktop) editPreset(p config.Preset) {
	d.presetName, d.editingPreset, d.presetModal, d.presetReplace = localizedPresetName(d.locale(), p), p.ID, true, false
}

func (d *desktop) deletePreset(id string) {
	if !customPreset(id) {
		return
	}
	m := d.model
	p := m.Preferences.Clone()
	p.Presets = slices.DeleteFunc(p.Presets, func(pr config.Preset) bool { return pr.ID == id })
	if p.DefaultPreset == id {
		p.DefaultPreset = ""
	}
	if d.chosenPreset == id {
		d.chosenPreset = ""
	}
	m.SetPreferences(p)
}

func (d *desktop) savePreset() {
	m := d.model
	name := strings.TrimSpace(d.presetName)
	if name == "" || len(name) > 180 {
		m.Error = l10n.New("error.invalidPresetName")
		return
	}
	p := m.Preferences.Clone()
	for _, pr := range p.Presets {
		if pr.ID != d.editingPreset && (strings.EqualFold(pr.Name, name) || strings.EqualFold(localizedPresetName(d.locale(), pr), name)) {
			m.Error = l10n.New("error.duplicatePreset")
			return
		}
	}
	id := d.editingPreset
	creating := id == ""
	if creating {
		if len(p.Presets) >= maxPresets {
			m.Error = l10n.Error("error.presetLimit", l10n.Args{"limit": maxPresets})
			return
		}
		var bytes [8]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			m.Error = err
			return
		}
		id = hex.EncodeToString(bytes[:])
	}
	preset := config.Preset{ID: id, Name: name, Image: m.Image, Naming: m.Naming}
	if old, ok := p.Preset(id); ok {
		if !d.presetReplace {
			preset.Image, preset.Naming = old.Image, old.Naming
		}
		if old.HasDefaultName() && localizedPresetName(d.locale(), old) == name {
			preset.Name = old.Name
		}
	}
	if err := preset.Image.Validate(); err != nil {
		m.Error = err
		return
	}
	if _, err := preset.Naming.Name("sample.jpg", preset.Image.Format.Extension(), 0, time.Now()); err != nil {
		m.Error = err
		return
	}
	i := slices.IndexFunc(p.Presets, func(pr config.Preset) bool { return pr.ID == id })
	if i < 0 {
		p.Presets = append(p.Presets, preset)
	} else {
		p.Presets[i] = preset
	}
	d.chosenPreset = id
	d.presetModal = false
	m.SetPreferences(p)
	if creating || d.presetReplace {
		m.MarkSavedPreset(preset)
	}
}

func (d *desktop) presetInspector(c *ui.Context) {
	m := d.model
	p, ok := m.Preferences.Preset(d.chosenPreset)
	if !ok {
		compactInspector(c, tr(c, "nav.presets"), nil, func() { note(c, tr(c, "preset.selectHint")) })
		return
	}
	compactInspector(c, presetDisplayName(c, p), func() {
		if smallIcon(c, lucide.Pencil(), tr(c, "preset.edit"), false, false) {
			d.editPreset(p)
		}
		if customPreset(p.ID) && smallIcon(c, lucide.Trash2(), tr(c, "preset.delete"), false, false) {
			d.deletePreset(p.ID)
		}
	}, func() {
		ui.Column(c).Children(func() {
			detailRow(c, tr(c, "nav.watermark"), markText(c, p.Image))
			detailRow(c, tr(c, "crop.resizeTab"), resizeText(c, p.Image.Resize))
			if r := p.Image.Resize; r.Enabled && r.Mode == imaging.Exact {
				detailRow(c, tr(c, "resize.fitMode"), fitLabel(c, r.Fit))
			}
			detailRow(c, tr(c, "nav.format"), formatText(c, p.Image))
			detailRow(c, tr(c, "rename.naming"), choose(p.Naming.KeepOriginal, tr(c, "rename.keepOriginal"), p.Naming.Pattern()))
		})
		line(c)
		def := m.Preferences.DefaultPreset == p.ID
		old := def
		switchRow(c, tr(c, "preset.makeDefault"), nil, &def, false)
		if old != def {
			d.setDefaultPreset(choose(def, p.ID, ""))
		}
		if action.Button(c, tr(c, "preset.apply"), action.ButtonProps{Variant: action.Primary}).AlignSelf(ui.Stretch).
			Disabled(m.AppliedPreset == p.ID && !m.SelectionHasMixedPresets()).Clicked() {
			m.ApplyPreset(p)
		}
	})
}

func (d *desktop) settingsPage(c *ui.Context) {
	m := d.model
	p := m.Preferences
	selectWidth := float32(154)
	if c.Locale() != "" && c.Locale() != l10n.Default {
		selectWidth = 256
	}
	contentScroll(c.Key("settings-scroll"), 864, 24, func() {
		ui.Column(c).Padding(10, 0, 0, 0).Gap(24).Children(func() {
			settingsGroup(c, tr(c, "settings.appearance"), func() {
				setting(c, tr(c, "settings.theme"), tr(c, "settings.themeHint"), func() {
					names := [...]string{"system", "light", "dark"}
					t := max(0, slices.Index(names[:], p.Theme))
					action.Segmented(c, &t, []string{tr(c, "settings.followSystem"), tr(c, "settings.light"), tr(c, "settings.dark")}, action.SegmentedProps{Size: theme.Small})
					p.Theme = names[t]
				})
				line(c)
				setting(c, tr(c, "settings.language"), tr(c, "settings.languageHint"), func() {
					value := c.Locale()
					langs := l10n.Languages()
					choices := make([]field.SelectOption, 0, len(langs))
					for _, lang := range langs {
						choices = append(choices, field.SelectOption{Value: lang.Tag, Label: lang.Name})
					}
					field.Select(c.Key("language"), &value, field.SelectProps{Options: choices}).Width(184).Label(tr(c, "settings.interfaceLanguage"))
					if l10n.Supported(value) && value != c.Locale() {
						p.Language = value
					}
				})
			})
			settingsGroup(c, tr(c, "settings.defaultOutput"), func() {
				setting(c, tr(c, "nav.format"), tr(c, "settings.formatHint"), func() {
					value := string(p.Format)
					field.Select(c, &value, field.SelectProps{Options: formatOptions}).Width(selectWidth).Label(tr(c, "settings.defaultFormat"))
					p.Format = imaging.Format(value)
				})
				line(c)
				setting(c, tr(c, "settings.folder"), tr(c, "settings.outputFolderHint"), func() {
					ui.Row(c).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
						if directoryButton(c, p.Output.Directory, d.dialog) {
							d.pickDirectory(true)
						}
						if p.Output.Directory != "" && smallIcon(c, lucide.X(), tr(c, "settings.clearOutputFolder"), false, d.dialog) {
							p.Output.Directory = ""
						}
					})
				})
				line(c)
				setting(c, tr(c, "settings.filenameConflict"), tr(c, "settings.conflictHint"), func() {
					value := string(p.Output.Conflict)
					field.Select(c, &value, field.SelectProps{Options: []field.SelectOption{
						{Value: string(files.Rename), Label: tr(c, "output.autoRename")}, {Value: string(files.Skip), Label: tr(c, "output.skipExisting")},
					}}).Width(selectWidth).Label(tr(c, "settings.defaultConflictPolicy"))
					p.Output.Conflict = files.Conflict(value)
				})
			})
			settingsGroup(c, tr(c, "settings.batch"), func() {
				setting(c, tr(c, "settings.defaultWorkers"), tr(c, "settings.workersHint"), func() {
					ui.Column(c).Width(selectWidth).Children(func() { numberField(c, tr(c, "settings.defaultWorkers"), &p.Workers, 1, batch.MaxWorkers, false) })
				})
			})
			settingsGroup(c, tr(c, "settings.about"), func() {
				tk := theme.TokensOf(c)
				ui.Row(c).Height(60).PaddingX(16).Gap(10).AlignItems(ui.Center).Children(func() {
					brandIcon(c)
					strong(c, tr(c, "app.name"))
					ui.Box(c).Grow(1)
					mono(c, Version).TextColor(tk.TextSecondary).Tooltip(tr(c, "settings.version"))
					d.updateButton(c)
				})
				d.automaticUpdateSetting(c)
				d.aboutLinks(c)
				d.aboutCredits(c)
			})
			if m.PreferenceError != nil {
				feedback.Alert(c, tr(c, "settings.preferencesFailure"), feedback.AlertProps{Type: feedback.Error, Description: errText(c, m.PreferenceError)})
			}
		})
	})
	d.applySettings(p)
}

func (d *desktop) applySettings(p config.Preferences) {
	m := d.model
	old := m.Preferences
	if p.Theme != old.Theme {
		mygo.Theme.SetSource(mygo.ThemeSource(p.Theme))
	}
	if p.Language != old.Language || p.Theme != old.Theme || p.Format != old.Format || p.Output != old.Output || p.Workers != old.Workers {
		m.SetPreferences(p)
	}
	if p.Language != old.Language && d.win != nil {
		mygo.App.SetLocale(p.Language)
	}
}

func directoryButton(c *ui.Context, path string, disabled bool) bool {
	tk := theme.TokensOf(c)
	label, value, help := tr(c, "settings.changeOutputFolder"), path, path
	if path == "" {
		label, value, help = tr(c, "settings.chooseOutputFolder"), tr(c, "settings.chooseFolderPlaceholder"), tr(c, "settings.chooseOutputFolder")
	}
	b := ui.ButtonBase(c).MinWidth(154).MaxWidth(440).Shrink(1).MinHeight(37).Padding(8, 10).
		Gap(8).Radius(tk.BorderRadius).AlignItems(ui.Center).Justify(ui.Start).
		Label(label).Tooltip(help).Cursor(ui.CursorPointer).Disabled(disabled)
	bg, fg := tk.FillQuaternary, tk.TextSecondary
	switch {
	case disabled:
		fg = tk.TextDisabled
	case b.Pressed():
		bg = tk.FillSecondary
	case b.Hovered():
		bg = tk.FillTertiary
	}
	b.Background(bg).Children(func() {
		icon(c, lucide.FolderOpen(), 15, fg)
		ui.Text(c, value).Wrap().MaxLines(3).MinWidth(0).Grow(1).FontSize(12).TextColor(fg)
	})
	return b.Clicked()
}

func settingsGroup(c *ui.Context, title string, body func()) {
	tk := theme.TokensOf(c)
	ui.Column(c).Gap(10).Children(func() {
		strong(c, title).FontSize(15)
		ui.Column(c).Radius(tk.BorderRadiusLG).Border(1, tk.BorderSecondary).Children(body)
	})
}

func setting(c *ui.Context, title, desc string, control func()) {
	ui.Row(c).MinHeight(60).Padding(10, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).Wrap().MaxLines(2).MinWidth(0).Tooltip(desc)
		ui.Box(c).Grow(1).MinWidth(0)
		control()
	})
}
