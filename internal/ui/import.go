package ui

import (
	"embed"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (d *desktop) importWelcome(c *ui.Context, dragging bool) {
	m, tk := d.model, theme.TokensOf(c)
	loading := m.Scanning || len(m.Sources) > 0
	dragging = dragging && !loading
	ui.Column(c).Grow(1).MinHeight(0).Center().Background(choose(dragging, tk.FillQuaternary, tk.BgContainer)).Children(func() {
		ui.Row(c).Gap(20).AlignItems(ui.Center).Children(func() {
			welcomeStack(c.Key("photos"), dragging, loading)
			width := float32(250)
			if c.Locale() != "" && c.Locale() != "zh-Hans" {
				width = 340
			}
			ui.Column(c).Width(width).Gap(15).Children(func() {
				title := choose(dragging, "import.release", "import.dropImages")
				if loading {
					title = "import.scanning"
				}
				ui.Text(c, tr(c, title)).FontSize(25).FontWeight(600).TextColor(tk.Text).Wrap()
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if button(c, tr(c, "import.chooseImages"), lucide.ImagePlus(), action.Primary, d.dialog || loading) {
						d.pickImport(false)
					}
					d.welcomeFolderButton(c.Key("folder"), d.dialog || loading)
				})
			})
		})
		ui.Column(c).MaxWidth(510).Children(func() {
			if d.importAttempted && !dragging && !loading {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
					icon(c, lucide.CircleAlert(), 14, tk.ErrorText)
					msg := choose(m.Scan.Errors > 0, "import.unreadable", "import.noImages")
					if m.Scan.Errors == 0 && m.Scan.SkippedOutputFolder {
						msg = "import.outputFolderSkipped"
					}
					text := tr(c, msg)
					if len(m.Scan.Issues) == 1 && m.Scan.Errors == 1 {
						issue := m.Scan.Issues[0]
						text = filepath.Base(issue.Path) + ": " + errText(c, issue.Err)
					}
					ui.Text(c, text).FontSize(tk.FontSizeSM).TextColor(tk.ErrorText).Wrap().MinWidth(0).Shrink(1)
					if m.Scan.Errors > 1 && styledButtonSize(c, tr(c, "import.details"), nil, action.Text, theme.Small, d.dialog, false).Clicked() {
						d.showScanIssues(c)
					}
				})
			}
		})
	})
	ui.Row(c).Height(48).Shrink(0).PaddingX(16).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Center).Children(func() {
			if styledButtonSize(c, tr(c, "nav.settings"), lucide.Settings(), action.Text, theme.Small, d.dialog, false).Clicked() {
				d.navigate(settings)
			}
		})
		d.queueUndoAction(c)
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End).Gap(6).AlignItems(ui.Center).PaddingX(4).Children(func() {
			icon(c, lucide.ShieldCheck(), 14, tk.TextSecondary)
			muted(c, tr(c, "import.localOnly")).MinWidth(0).Shrink(1)
		})
	})
}

// Both hit targets share one outline. The popup sizes to its localized item.
func (d *desktop) welcomeFolderButton(c *ui.Context, disabled bool) {
	tk := theme.TokensOf(c)
	group := ui.Row(c).Shrink(0).Height(32).AlignItems(ui.Center).Radius(tk.BorderRadiusSM).Border(1, tk.BorderSecondary).Background(tk.BgContainer)
	group.Children(func() {
		pick := styledButton(c.Key("choose"), tr(c, "import.chooseFolder"), lucide.Folder(), action.Text, disabled, false)
		pick.Height(30).PaddingX(9).Radius(tk.BorderRadiusSM, 0, 0, tk.BorderRadiusSM)
		if pick.Clicked() {
			d.pickImport(true)
		}
		ui.Box(c).Size(1, 14).Background(tk.BorderSecondary)
		// This menu belongs only to the welcome view, even when a batch is imported while it is open.
		open := ui.Local(group, "options", func() bool { return false })
		if disabled {
			*open = false
		}
		menu := ui.MenuBase(c.Key("options"), open)
		arrow := menu.Trigger.Label(tr(c, "import.folderOptions")).Tooltip(tr(c, "import.folderOptions")).Disabled(disabled)
		action.StyleButton(c, arrow, "", action.ButtonProps{Variant: action.Text})
		arrow.Size(25, 30).Padding(0).Center().Radius(0, tk.BorderRadiusSM, tk.BorderRadiusSM, 0)
		if *open {
			arrow.Background(tk.FillQuaternary)
		}
		arrow.Children(func() { icon(c, lucide.ChevronDown(), 12, tk.TextSecondary) })
		menu.Popup(func(panel ui.Element) {
			panel.AttachTo(group, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(5, 0, 0, 0).Padding(4).Radius(tk.BorderRadius).Border(1, tk.BorderSecondary).Background(tk.BgElevated)
			tk.Elevate(panel, theme.BoxShadowSecondary)
			panel.Children(func() {
				checked := d.model.Preferences.Recursive
				label := tr(c, "import.recursive")
				item := menu.CheckItem(checked).Label(label).Height(29).AlignItems(ui.Center).PaddingX(8).Gap(6).Radius(tk.BorderRadiusSM)
				if item.Highlighted() {
					item.Background(tk.FillTertiary)
				}
				item.Children(func() {
					ui.Box(c).Size(14, 14).Center().Children(func() {
						if checked {
							icon(c, lucide.Check(), 13, tk.Text)
						}
					})
					ui.Text(c, label).SingleLine().FontSize(tk.FontSizeSM).TextColor(tk.Text)
				})
				if item.Clicked() {
					d.setRecursive(!checked)
				}
			})
		})
	})
}

func (d *desktop) importSettings(c *ui.Context) {
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).Children(func() {
		ui.Row(c).Height(52).Shrink(0).PaddingX(18).Gap(8).AlignItems(ui.Center).Children(func() {
			if smallIcon(c, lucide.ArrowLeft(), tr(c, "import.back"), false, d.dialog) {
				d.navigate(importHome)
			}
			strong(c, tr(c, "nav.settings")).FontSize(16)
		})
		line(c)
		d.settingsPage(c.Key("settings"))
	})
}

//go:embed assets/welcome/*.jpg
var welcomePhotos embed.FS

// Keep all three prints in native view state; drag events only move their frames.
func welcomeStack(c *ui.Context, dragging, loading bool) {
	stack := ui.Box(c).Size(181, 135).Shrink(0).IgnoreInput(true)
	photos := ui.Local(stack, "prints", func() [3]*ui.Bitmap {
		var out [3]*ui.Bitmap
		for i, name := range []string{"left", "right", "front"} {
			data, _ := welcomePhotos.ReadFile("assets/welcome/" + name + ".jpg")
			out[i], _ = ui.DecodeBitmap(data)
		}
		stack.Jump("spread", .78)
		return out
	})
	spread := float32(1)
	if !c.Preferences().ReduceMotion {
		target := choose(loading, float32(.65), choose(dragging, float32(1.42), float32(1)))
		spread = stack.AnimateTo("spread", target, ui.Animation{Duration: 240 * time.Millisecond})
	}
	stack.Children(func() {
		for i, p := range *photos {
			x := [...]float32{-18, 18, 0}[i]
			angle := [...]float32{-13, 11, -4}[i]
			y := [...]float32{19, 20, 27}[i]
			ui.Box(c.Key(i)).Absolute().Left(28+x*spread).Top(y).Size(125, 89).Padding(4).Radius(6).Background(ui.RGB(252, 252, 251)).Shadow(0, 3, 9, 0, ui.RGBA(0, 0, 0, 38)).Rotate(angle * spread).Children(func() {
				if p != nil {
					ui.Image(c, p).Fill().Radius(3).Clip().Fit(ui.Cover)
				}
			})
		}
	})
}
