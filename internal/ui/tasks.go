package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"imageflow/internal/app"
	"imageflow/internal/batch"
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

func elapsed(p batch.Progress) string {
	t := p.Finished
	if t.IsZero() {
		t = time.Now()
	}
	sec := max(0, int(t.Sub(p.Started).Seconds()))
	if sec >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec/60%60, sec%60)
	}
	return fmt.Sprintf("%02d:%02d", sec/60, sec%60)
}

func (d *desktop) taskPage(c *ui.Context) {
	m := d.model
	if m.Progress.Total == 0 {
		empty(c, tr(c, "task.empty"), tr(c, "task.emptyHint"), lucide.ListChecks())
		return
	}
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).Center().Padding(24, 24, 68, 24).Children(func() {
		if m.Working {
			d.runningTask(c)
		} else {
			d.completedTask(c)
		}
	})
}

func (d *desktop) runningTask(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	p := m.Progress
	ui.Column(c).WidthPercent(100).MaxWidth(680).Gap(12).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil)
			strong(c, choose(m.Cancelling, tr(c, "task.cancelling"), tr(c, "task.processing"))).FontSize(24)
			ui.Box(c).Grow(1)
			strong(c, fmt.Sprintf("%d%%", p.Percent())).FontSize(30).FontFeatures("tnum")
		})
		feedback.Progress(c, float32(p.Percent()), feedback.ProgressProps{Color: tk.Primary, Size: theme.Large})
		mono(c, trf(c, "task.doneOfTotal", l10n.Args{"done": p.Done(), "total": p.Total}))
		taskStats(c, p, true)
		ui.Row(c).Height(62).Margin(10, 0, 0, 0).PaddingX(10).Gap(10).AlignItems(ui.Center).
			Background(tk.BgLayout).Radius(tk.BorderRadiusLG).Children(func() {
			if len(p.Current) == 0 {
				icon(c, lucide.Clock(), 17, tk.TextSecondary)
				muted(c, choose(m.Cancelling, tr(c, "task.waitingToCancel"), tr(c, "task.planningOrWaiting")))
				return
			}
			path := p.Current[0].Path
			if m.Thumbnail(path).Image != nil {
				d.thumb(c, path, 56, 42)
			} else {
				ui.Box(c).Size(56, 42).Center().Background(tk.FillQuaternary).Radius(tk.BorderRadiusSM).Children(func() {
					icon(c, lucide.FileImage(), 20, tk.TextSecondary)
				})
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				muted(c, tr(c, "task.currentFile"))
				strong(c, baseName(c, path)).Tooltip(path)
			})
			if len(p.Current) > 1 {
				muted(c, trf(c, "task.moreCurrent", l10n.Args{"count": len(p.Current) - 1}))
			}
		})
	})
}

func (d *desktop) completedTask(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	p := m.Progress
	svg, fg, bg := lucide.Check(), tk.SuccessText, tk.SuccessBg
	label := tr(c, "task.completed")
	switch {
	case p.Cancelled > 0:
		svg, fg, bg = lucide.OctagonPause(), tk.WarningText, tk.WarningBg
		label = tr(c, "task.cancelledTitle")
	case p.Success == 0 && p.Failure > 0:
		svg, fg, bg = lucide.CircleAlert(), tk.ErrorText, tk.ErrorBg
		label = tr(c, "task.noSuccess")
	}
	ui.Column(c).WidthPercent(100).MaxWidth(720).AlignItems(ui.Center).Gap(13).Children(func() {
		ui.Box(c).Size(68, 68).Radius(34).Center().Background(bg).Children(func() { icon(c, svg, 35, fg) })
		strong(c, label).FontSize(24)
		taskStats(c, p, false)
		if p.Failure != len(m.Failures) {
			muted(c, trf(c, "task.unresolvedCount", l10n.Args{"count": len(m.Failures)}))
		}
		ui.Row(c).Margin(13, 0, 0, 0).Gap(8).AlignItems(ui.Center).Justify(ui.Center).Wrap().Children(func() {
			if button(c, tr(c, "output.openFolder"), lucide.FolderOpen(), action.Primary, m.Report.Directory == "") {
				d.openOutput(m.Report.Directory)
			}
			if button(c, tr(c, "task.viewFailures"), nil, action.Default, len(m.Failures) == 0) {
				d.navigate(failures)
			}
			if button(c, tr(c, "task.runAgain"), lucide.RotateCcw(), action.Text, len(m.Sources) == 0 || m.Scanning) {
				d.start(nil)
			}
		})
		if m.Report.Directory != "" {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Margin(5, 0, 0, 0).Children(func() {
				icon(c, lucide.Folder(), 14, tk.TextTertiary)
				muted(c, m.Report.Directory).MinWidth(0).Shrink(1).Tooltip(m.Report.Directory)
			})
		}
	})
}

func taskStats(c *ui.Context, p batch.Progress, running bool) {
	tk := theme.TokensOf(c)
	type stat struct {
		label string
		n     int
		color ui.Color
	}
	metrics := []stat{{tr(c, "task.successCount"), p.Success, tk.SuccessText}, {tr(c, "task.failureCount"), p.Failure, tk.ErrorText}, {tr(c, "task.skippedCount"), p.Skipped, tk.Text}}
	if running {
		metrics = append(metrics, stat{tr(c, "task.remaining"), p.Remaining, tk.Text}, stat{tr(c, "task.running"), p.Running, tk.Text})
	}
	metrics = append(metrics, stat{tr(c, "task.cancelledCount"), p.Cancelled, tk.Text})
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
		for i, s := range metrics {
			ui.Row(c.Key(s.label)).Height(25).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
				if i > 0 {
					ui.Box(c).Size(1, 13).Background(tk.BorderSecondary).Margin(0, 5, 0, 0)
				}
				strong(c, fmt.Sprint(s.n)).FontSize(16).FontFeatures("tnum").TextColor(s.color)
				muted(c, s.label)
			})
		}
	})
}

func (d *desktop) taskInspector(c *ui.Context) {
	job := d.model.Job
	compactInspector(c, tr(c, "task.parameters"), nil, func() {
		ui.Column(c).Children(func() {
			if job.Mixed {
				detailRow(c, tr(c, "scope.edits"), tr(c, "scope.perPhoto"))
			} else {
				detailRow(c, tr(c, "nav.presets"), choose(job.Preset.ID != "", presetDisplayName(c, job.Preset), tr(c, "preset.custom")))
				detailRow(c, tr(c, "crop.resizeTab"), resizeText(c, job.Image.Resize))
				detailRow(c, tr(c, "nav.format"), formatText(c, job.Image))
			}
			detailRow(c, tr(c, "task.workers"), fmt.Sprint(job.Workers))
			detailRow(c, tr(c, "output.conflictFiles"), choose(job.Output.Conflict == files.Skip, tr(c, "output.skipExisting"), tr(c, "output.autoRename")))
		})
		line(c)
		strong(c, tr(c, "output.folder"))
		pathLine(c, job.Output.Directory, lucide.Folder())
	})
}

func (d *desktop) openOutput(path string) {
	if path == "" {
		return
	}
	go func() {
		if err := mygo.Shell.OpenPath(path); err != nil {
			mygo.RunOnMain(func() { d.model.Error = err; d.redraw() })
		}
	}()
}

func (d *desktop) canRetry(path string, all bool) bool {
	return !d.skippedFailures[path] && (all || d.errorSelected[path])
}

func (d *desktop) retryableCount() int {
	n := 0
	for _, r := range d.model.Failures {
		if d.canRetry(r.Entry.Source.ID(), true) {
			n++
		}
	}
	return n
}

func (d *desktop) retrySelected(all bool) {
	items := d.model.Failures
	src := make([]files.Source, 0, len(items))
	for _, r := range items {
		if d.canRetry(r.Entry.Source.ID(), all) {
			src = append(src, r.Entry.Source)
		}
	}
	if len(src) > 0 {
		d.start(src)
	}
}

// Skipping from the error view excludes the file from retries in the current
// batch. The failure, including the log entry and its outcome, stays intact.
func (d *desktop) skipSelectedFailures() {
	if d.skippedFailures == nil {
		d.skippedFailures = make(map[string]bool)
	}
	for _, f := range d.model.Failures {
		id := f.Entry.Source.ID()
		if d.errorSelected[id] {
			d.skippedFailures[id] = true
			delete(d.errorSelected, id)
		}
	}
}

const errorPerPage = 12

func (d *desktop) errorPageView(c *ui.Context) {
	items, tk := d.model.Failures, theme.TokensOf(c)
	if len(items) == 0 {
		empty(c, tr(c, "failure.empty"), tr(c, "failure.emptyHint"), lucide.CircleCheck())
		return
	}
	if !slices.ContainsFunc(items, func(r app.Failure) bool { return r.Entry.Source.ID() == d.errorFocus }) {
		d.errorFocus = items[0].Entry.Source.ID()
	}
	start, end := pageRange(len(items), errorPerPage, &d.errorPage)
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).Padding(18, 24, 0, 24).Gap(12).Children(func() {
		ui.Row(c).MinHeight(38).Shrink(0).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
			canRetry := false
			for path := range d.errorSelected {
				canRetry = canRetry || d.canRetry(path, false)
			}
			if button(c, tr(c, "failure.retrySelected"), lucide.RotateCcw(), action.Filled, !canRetry) {
				d.retrySelected(false)
			}
			if button(c, tr(c, "failure.excludeSelected"), lucide.SkipForward(), action.Text, !canRetry) {
				d.skipSelectedFailures()
			}
			if len(d.skippedFailures) > 0 && button(c, tr(c, "failure.restoreRetry"), nil, action.Text, false) {
				clear(d.skippedFailures)
			}
			ui.Box(c).Grow(1)
			if button(c, tr(c, "failure.exportLog"), lucide.Download(), action.Text, d.dialog) {
				d.saveErrors(items)
			}
		})
		ui.Column(c).Grow(1).Basis(0).MinHeight(0).Children(func() {
			ui.Row(c).Height(38).Shrink(0).PaddingX(12).Gap(10).Background(tk.BgLayout).Radius(tk.BorderRadiusSM).AlignItems(ui.Center).Children(func() {
				all := len(d.errorSelected) == len(items)
				v := all
				choice.Checkbox(c, &v, "", choice.CheckboxProps{Indeterminate: len(d.errorSelected) > 0 && !all}).Label(tr(c, "failure.selectAll"))
				if v != all {
					clear(d.errorSelected)
					if v {
						if d.errorSelected == nil {
							d.errorSelected = make(map[string]bool)
						}
						for _, r := range items {
							d.errorSelected[r.Entry.Source.ID()] = true
						}
					}
				}
				muted(c, tr(c, "common.filename")).Grow(3).Basis(0).MinWidth(0)
				muted(c, tr(c, "common.location")).Grow(2).Basis(0).MinWidth(0)
				muted(c, tr(c, "failure.reason")).Grow(3).Basis(0).MinWidth(0)
			})
			layout.ScrollArea(c.Key(d.errorPage), layout.ScrollAreaProps{}, func() {
				ui.Column(c).Children(func() {
					for _, r := range items[start:end] {
						d.errorRow(c.Key(r.Entry.Source.ID()), r)
					}
				})
			}).Grow(1).Basis(0).MinHeight(0).AlignSelf(ui.Stretch)
		})
	})
}

func (d *desktop) errorRow(c *ui.Context, r app.Failure) {
	tk := theme.TokensOf(c)
	path, id := r.Entry.Source.Path, r.Entry.Source.ID()
	name := sourceName(c, r.Entry.Source)
	row := ui.Row(c).Height(54).PaddingX(12).Gap(10).AlignItems(ui.Center).Radius(tk.BorderRadiusSM)
	if d.errorFocus == id || d.errorSelected[id] {
		row.Background(tk.FillQuaternary)
	}
	row.Children(func() {
		v := d.errorSelected[id]
		choice.Checkbox(c, &v, "", choice.CheckboxProps{}).Label(trf(c, "common.selectFile", l10n.Args{"name": name}))
		if v != d.errorSelected[id] {
			d.errorFocus = id
			if v {
				if d.errorSelected == nil {
					d.errorSelected = make(map[string]bool)
				}
				d.errorSelected[id] = true
			} else {
				delete(d.errorSelected, id)
			}
		}
		b := ui.ButtonBase(c).Grow(3).Basis(0).MinWidth(0).Height(52).Gap(7).AlignItems(ui.Center).Justify(ui.Start).Label(name).Tooltip(path)
		b.Children(func() {
			icon(c, lucide.FileImage(), 15, tk.TextSecondary)
			strong(c, name).MinWidth(0).Shrink(1)
		})
		if b.Clicked() {
			d.errorFocus = id
		}
		dir := filepath.Dir(path)
		muted(c, baseName(c, dir)+"/").Grow(2).Basis(0).MinWidth(0).Tooltip(dir)
		reason := failureReason(c, r)
		ui.Row(c).Grow(3).Basis(0).MinWidth(0).Gap(6).AlignItems(ui.Center).Tooltip(reason).Children(func() {
			ui.Box(c).Size(6, 6).Shrink(0).Radius(3).Background(tk.Error)
			if d.skippedFailures[id] {
				muted(c, tr(c, "failure.retryExcluded")).MinWidth(0).Shrink(1)
			} else {
				ui.Text(c, reason).SingleLine().MinWidth(0).Shrink(1)
			}
		})
	})
	line(c)
}

func failureReason(c *ui.Context, r app.Failure) string {
	if r.Err != nil {
		return errText(c, r.Err)
	}
	return tr(c, "task.failed")
}

func (d *desktop) errorInspector(c *ui.Context) {
	items := d.model.Failures
	i := slices.IndexFunc(items, func(r app.Failure) bool { return r.Entry.Source.ID() == d.errorFocus })
	if i < 0 {
		compactInspector(c, tr(c, "failure.inspector"), nil, func() { note(c, tr(c, "failure.selectHint")) })
		return
	}
	r := items[i]
	path := r.Entry.Source.Path
	compactInspector(c, sourceName(c, r.Entry.Source), nil, func() {
		feedback.Alert(c, tr(c, r.Stage), feedback.AlertProps{Type: feedback.Error, Description: failureReason(c, r)})
		if d.skippedFailures[r.Entry.Source.ID()] {
			note(c, tr(c, "failure.retryExcludedHint"))
		}
		strong(c, tr(c, "failure.sourceFile"))
		pathLine(c, path, lucide.FileImage())
		strong(c, tr(c, "failure.destinationFile"))
		target := ""
		if r.Directory != "" && r.Entry.Relative != "" {
			target = filepath.Join(r.Directory, r.Entry.Relative)
		}
		pathLine(c, target, lucide.FolderOutput())
		ui.Row(c).Children(func() {
			if button(c, tr(c, "output.changeFolder"), lucide.FolderOpen(), action.Default, false) {
				d.pickDirectory(false)
			}
		})
	})
}

func (d *desktop) saveErrors(items []app.Failure) {
	if d.dialog {
		return
	}
	d.dialog = true
	lang := d.locale()
	var text strings.Builder
	for _, r := range items {
		fmt.Fprintf(&text, "%s\t%s\t%s\t%s\n", r.Finished.Format(time.RFC3339), r.Entry.Source.Path, l10n.T(lang, r.Stage), l10n.ErrorText(lang, r.Err))
	}
	opts := mygo.SaveDialogOptions{Parent: d.win, Title: d.t("failure.exportLogTitle"), DefaultPath: d.t("failure.logFilename"), ButtonLabel: d.t("common.save")}
	go func() {
		path, err := mygo.Dialog.Save(opts)
		if err == nil && path != "" {
			var f *os.File
			f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err == nil {
				_, writeErr := f.WriteString(text.String())
				closeErr := f.Close()
				err = writeErr
				if err == nil {
					err = closeErr
				}
				if err != nil {
					_ = os.Remove(path)
				}
			} else if os.IsExist(err) {
				err = l10n.New("error.logAlreadyExists")
			}
		}
		mygo.RunOnMain(func() {
			d.dialog = false
			if err != nil {
				d.model.Error = err
			}
			d.redraw()
		})
	}()
}
