// Package app coordinates the application. Its Model is owned by the UI thread;
// worker completions are marshalled through the supplied dispatcher.
package app

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"imageflow/internal/batch"
	"imageflow/internal/config"
	"imageflow/internal/crop"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/preview"
	"imageflow/internal/series"
)

type ViewMode int

const (
	Before ViewMode = iota
	After
	Encoded
)

type Thumb struct {
	Image image.Image
	Info  imaging.Info
	Err   error
}

type planRequest struct {
	src   []files.Source
	out   files.Settings
	rules files.Rules
	ext   string
	date  time.Time
}
type planResult struct {
	Value files.Plan
	Err   error
}

type Job struct {
	Image   imaging.Options
	Naming  files.Rules
	Output  files.Settings
	Workers int
	Date    time.Time
	Preset  config.Preset
	Mixed   bool
}

type Model struct {
	Failures               []Failure
	failureJob             Job
	Sources                []files.Source
	Selected               string
	EditSelection          map[string]bool // The previewed photo is always in this set.
	ScopeRevision          uint64
	presetRevision         uint64
	mixedPresets           bool
	defaults               files.PhotoSettings
	PreviewPart            int
	Mode                   ViewMode
	Image                  imaging.Options
	Naming                 files.Rules
	Output                 files.Settings
	Preferences            config.Preferences
	AppliedPreset          string
	PreferenceError        error
	LogoPending            bool
	logoTargets            map[string]bool
	LastAddedLogo          string
	LogoImportRevision     uint64
	CertificationLoading   bool
	CertificationImporting bool
	certificationTargets   map[string]bool
	CertificationError     error
	PrefsPending           bool
	Plan                   files.Plan // Display snapshot while Planning is true; otherwise the current plan.
	PlanErr                error
	Planning               bool
	Preview                imaging.Preview
	PreviewErr             error
	Rendering              bool
	Scanning               bool
	Working                bool
	Cancelling             bool
	Scan                   batch.ScanReport
	Job                    Job
	Progress               batch.Progress
	Report                 batch.Report
	Error                  error
	Collage                Collage

	ctx            context.Context
	cancel         context.CancelFunc
	dispatch       func(func())
	changed        func()
	wg             sync.WaitGroup
	jobCancel      context.CancelFunc
	pendingImports []string
	history        editHistory
	knownOutputs   []string
	preview        previewSession
	caption        captionSession
	collage        collageSession
	certification  certificationSession

	planner        *preview.Latest[planRequest, planResult]
	thumbnailer    *preview.Latest[[]string, map[string]Thumb]
	saver          *preview.Latest[config.Preferences, error]
	thumbs         map[string]Thumb
	thumbOrder     []string
	visible        []string
	configPath     string
	configReadOnly bool
	configDirty    bool
}

func New(dispatch func(func()), changed func()) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Model{ctx: ctx, cancel: cancel, dispatch: dispatch, changed: changed, thumbs: map[string]Thumb{}, PreviewPart: 1}
	path, err := config.Path()
	prefs := config.Defaults()
	if err == nil {
		prefs, err = config.Load(path)
	}
	m.Preferences, m.PreferenceError, m.configPath, m.configReadOnly = prefs, err, path, err != nil
	m.Image = imaging.Defaults()
	m.Image.Format = prefs.Format
	m.Naming = files.DefaultRules()
	m.Output = prefs.Output
	if p, ok := prefs.Preset(prefs.DefaultPreset); ok {
		m.Image, m.Naming, m.AppliedPreset = presetImage(p.Image), p.Naming, p.ID
	}
	m.defaults = m.focusSettings()
	m.EditSelection = make(map[string]bool)
	m.initPreview()
	m.initCaption()
	m.initCollage()
	m.planner = preview.New(ctx, 100*time.Millisecond, func(ctx context.Context, r planRequest) planResult {
		p, err := files.BuildPlan(ctx, r.src, r.out, r.rules, r.ext, r.date)
		return planResult{p, err}
	}, func(rev uint64, r planResult) {
		m.onUI(func() {
			if m.planner.IsCurrent(rev) {
				m.Plan, m.PlanErr, m.Planning = r.Value, r.Err, false
			}
		})
	})
	m.thumbnailer = preview.New(ctx, 45*time.Millisecond, func(ctx context.Context, paths []string) map[string]Thumb {
		out := make(map[string]Thumb, len(paths))
		for _, p := range paths {
			if ctx.Err() != nil {
				break
			}
			path, index, split := strings.Cut(p, "\x00")
			part := 1
			if split {
				part, _ = strconv.Atoi(index)
			}
			im, info, err := imaging.ThumbnailPart(ctx, path, part, 192)
			out[p] = Thumb{im, info, err}
		}
		return out
	}, func(rev uint64, r map[string]Thumb) {
		m.onUI(func() {
			if !m.thumbnailer.IsCurrent(rev) {
				return
			}
			for p, t := range r {
				if _, ok := m.thumbs[p]; !ok {
					m.thumbOrder = append(m.thumbOrder, p)
				}
				m.thumbs[p] = t
			}
			for len(m.thumbOrder) > 96 {
				delete(m.thumbs, m.thumbOrder[0])
				m.thumbOrder = m.thumbOrder[1:]
			}
			m.scheduleWarmup()
		})
	})
	m.saver = preview.New(ctx, 120*time.Millisecond, func(_ context.Context, p config.Preferences) error {
		return config.Save(path, p)
	}, func(rev uint64, err error) {
		m.onUI(func() {
			if m.saver.IsCurrent(rev) {
				m.PreferenceError, m.PrefsPending = err, false
				m.configDirty = err != nil
			}
		})
	})
	return m
}

func (m *Model) onUI(fn func()) {
	m.dispatch(func() {
		if m.ctx.Err() == nil {
			fn()
			m.changed()
		}
	})
}

func (m *Model) Close() error {
	m.cancel()
	m.preview.close()
	m.caption.work.Close()
	m.collage.tiles.Close()
	m.collage.marks.Close()
	m.planner.Close()
	m.thumbnailer.Close()
	m.saver.Close()
	m.wg.Wait()
	if m.configDirty && !m.configReadOnly {
		return config.Save(m.configPath, m.Preferences)
	}
	return nil
}

func (m *Model) NewBatch() {
	if m.Working || m.Scanning || m.LogoPending || m.CertificationImporting || m.Collage.Exporting {
		return
	}
	m.EndCollage()
	m.history = editHistory{next: m.history.next}
	m.LastAddedLogo = ""
	clear(m.EditSelection)
	m.ScopeRevision++
	m.Sources, m.Selected, m.AppliedPreset = nil, "", ""
	m.PreviewPart = 1
	m.resetThumbnails()
	m.preview.cache = baseCache{}
	m.Scan, m.Progress, m.Report = batch.ScanReport{}, batch.Progress{}, batch.Report{}
	m.Failures, m.failureJob = nil, Job{}
	m.Error = nil
	m.Image = imaging.Defaults()
	m.Image.Format = m.Preferences.Format
	m.Naming, m.Output = files.DefaultRules(), m.Preferences.Output
	if p, ok := m.Preferences.Preset(m.Preferences.DefaultPreset); ok {
		m.Image, m.Naming, m.AppliedPreset = presetImage(p.Image), p.Naming, p.ID
	}
	m.defaults = m.focusSettings()
	m.Mode = Before
	m.pruneLogoPreviews()
	m.requestPreview()
	m.RefreshPlan()
}

func (m *Model) resetThumbnails() {
	m.thumbnailer.Cancel()
	clear(m.thumbs)
	m.thumbOrder, m.visible = nil, nil
}

func (m *Model) removeThumbnails(paths map[string]bool) {
	m.thumbnailer.Cancel()
	m.visible = nil // Request the current page again on the next draw.
	m.thumbOrder = slices.DeleteFunc(m.thumbOrder, func(path string) bool {
		input, _, _ := strings.Cut(path, "\x00")
		if !paths[input] {
			return false
		}
		delete(m.thumbs, path)
		return true
	})
}

func (m *Model) Import(paths []string) {
	if len(paths) == 0 || m.Working {
		return
	}
	if m.Scanning {
		m.pendingImports = append(m.pendingImports, paths[:min(len(paths), max(0, batch.MaxFiles-len(m.pendingImports)))]...)
		return
	}
	m.Scanning = true
	src := slices.Clone(m.Sources)
	exclusions := slices.Clone(m.knownOutputs)
	for _, p := range []string{m.Output.Directory, m.Preferences.Output.Directory} {
		if p != "" {
			exclusions = append(exclusions, p)
		}
	}
	recursive := m.Preferences.Recursive
	paths = slices.Clone(paths)
	m.wg.Go(func() {
		for i, p := range exclusions {
			if canonical, err := filepath.EvalSymlinks(p); err == nil {
				exclusions[i] = canonical
			}
		}
		report := batch.Scan(m.ctx, paths, src, batch.ScanOptions{Recursive: recursive, Exclude: exclusions})
		m.onUI(func() {
			m.Scanning = false
			m.Scan = report
			start, before := len(m.Sources), m.historySelection()
			defaults := m.defaults
			for i := range report.Added {
				report.Added[i].Settings = &defaults
			}
			m.Sources = append(m.Sources, report.Added...)
			if m.Selected == "" && len(m.Sources) > 0 {
				m.Select(m.Sources[0].Path)
			}
			if len(report.Added) > 0 {
				m.recordQueueEdit(HistoryImport, contiguousSources(start, report.Added), before, m.historySelection())
				m.syncCollage()
			}
			m.RefreshPlan()
			if len(m.pendingImports) > 0 {
				more := m.pendingImports
				m.pendingImports = nil
				m.Import(more)
			}
		})
	})
}

func (m *Model) sourceIndex() int {
	return m.sourceIndexFor(m.Selected)
}

func (m *Model) sourceIndexFor(path string) int {
	return slices.IndexFunc(m.Sources, func(s files.Source) bool { return s.Path == path })
}

func (m *Model) SelectedSource() files.Source {
	if i := m.sourceIndex(); i >= 0 {
		return m.Sources[i]
	}
	return files.Source{}
}

func (m *Model) ApplyCrop(paths map[string]bool, region crop.Region) {
	if m.Working {
		return
	}
	var edits []sourceEdit[crop.Region]
	for i, s := range m.Sources {
		if paths[s.Path] && s.Crop != region {
			m.Sources[i].Crop = region
			edits = append(edits, sourceEdit[crop.Region]{i, s.Crop, region})
		}
	}
	if len(edits) > 0 {
		m.ScopeRevision++
		recordSources(m, HistoryCrop, 0, edits, func(s *files.Source) *crop.Region { return &s.Crop })
		m.requestPreview()
		m.syncCollage()
	}
}

func (m *Model) SetPreviewPart(part int) {
	if !m.Working && part != m.PreviewPart && part >= 1 && part <= m.SelectedSource().Series.Count {
		m.PreviewPart = part
		m.requestPreview()
	}
}

func (m *Model) SetSeriesSelection(selection series.Selection) {
	m.EditSeriesSelection(selection, 0)
}

func (m *Model) EditSeriesSelection(selection series.Selection, group HistoryGroup) {
	focus := m.SelectedSource()
	if m.Working || !focus.Series.Multiple() || selection == focus.Selection {
		return
	}
	var edits []sourceEdit[series.Selection]
	for i, s := range m.Sources {
		if (!m.EditSelection[s.Path] && s.Path != m.Selected) || s.Series.Kind != focus.Series.Kind || !s.Series.Multiple() {
			continue
		}
		next := s.Selection
		applyField(&next.Scope, focus.Selection.Scope, selection.Scope)
		applyField(&next.Range, focus.Selection.Range, selection.Range)
		applyField(&next.Separate, focus.Selection.Separate, selection.Separate)
		if next != s.Selection {
			edits = append(edits, sourceEdit[series.Selection]{i, s.Selection, next})
			m.Sources[i].Selection = next
		}
	}
	recordSources(m, HistorySeries, group, edits, func(s *files.Source) *series.Selection { return &s.Selection })
	m.ScopeRevision++
	m.requestPreview()
	m.schedulePlan(true)
}

func (m *Model) FirstSeriesIssue() (files.Source, error) {
	for _, s := range m.Sources {
		if err := s.Selection.Validate(s.Series); err != nil {
			return s, err
		}
	}
	return files.Source{}, nil
}

func (m *Model) SetMode(mode ViewMode) {
	if m.Mode != mode {
		m.Mode = mode
		m.requestPreview()
	}
}

func (m *Model) SetImage(o imaging.Options) {
	m.EditImage(o, HistoryLayers, 0)
}

func (m *Model) EditImage(o imaging.Options, action HistoryAction, group HistoryGroup) {
	m.EditImageFrom(m.Image, o, action, group)
}

// EditImageFrom accepts a control’s displayed baseline, including a mixed-value
// placeholder, so choosing the focused photo’s value can also unify a selection.
func (m *Model) EditImageFrom(before, after imaging.Options, action HistoryAction, group HistoryGroup) {
	if before == after {
		return
	}
	after = withContentWatermarks(after)
	changed := m.editPhotos(m.EditSelection, action, group, func(p files.PhotoSettings) files.PhotoSettings {
		next := patchImage(p.Image, before, after)
		if next != p.Image {
			p.Image, p.Preset = next, ""
		}
		return p
	})
	if changed {
		m.refreshImageEdit(before.Format != after.Format)
	}
}

func (m *Model) refreshImageEdit(formatChanged bool) {
	m.pruneLogoPreviews()
	m.requestPreview()
	if m.HasCollage() {
		m.requestCollageLayers()
	}
	if formatChanged {
		m.schedulePlan(true)
	}
}

func (m *Model) SetNaming(r files.Rules)                      { m.EditNaming(r, 0) }
func (m *Model) EditNaming(r files.Rules, group HistoryGroup) { m.EditNamingFrom(m.Naming, r, group) }
func (m *Model) EditNamingFrom(before, after files.Rules, group HistoryGroup) {
	if before == after {
		return
	}
	if m.editPhotos(m.EditSelection, HistoryRename, group, func(p files.PhotoSettings) files.PhotoSettings {
		next := patchNaming(p.Naming, before, after)
		if next != p.Naming {
			p.Naming, p.Preset = next, ""
		}
		return p
	}) {
		m.schedulePlan(before.KeepOriginal != after.KeepOriginal)
	}
}

func (m *Model) SetOutput(o files.Settings) {
	if !m.Working && m.Output != o {
		recordValue(m, HistoryOutput, 0, m.Output, o, func(m *Model, v files.Settings) { m.Output = v })
		sameDirectory := m.Output.Directory == o.Directory
		m.Output = o
		if sameDirectory {
			m.schedulePlan(true)
		} else {
			m.RefreshPlan()
		}
	}
}

func (m *Model) ApplyPreset(p config.Preset) {
	if m.Working || m.Collage.Exporting || m.LogoPending {
		return
	}
	settings := files.PhotoSettings{Image: presetImage(p.Image), Naming: p.Naming, Preset: p.ID}
	if m.editPhotos(m.EditSelection, HistoryPreset, 0, func(files.PhotoSettings) files.PhotoSettings { return settings }) {
		m.LastAddedLogo = ""
		m.refreshImageEdit(true)
	}
}

func withContentWatermarks(o imaging.Options) imaging.Options {
	o.Text.Enabled = strings.TrimSpace(o.Text.Text) != ""
	o.Logo.Enabled = strings.TrimSpace(o.Logo.Path) != ""
	for i := range o.AdditionalLogos {
		l := &o.AdditionalLogos[i]
		l.Enabled = strings.TrimSpace(l.Path) != ""
		if !l.Enabled {
			*l = imaging.LogoMark{}
		}
	}
	return o
}

// Older presets can keep content behind an off switch. Preserve their effect
// when applied without changing the stored preset or reactivating old content.
func presetImage(o imaging.Options) imaging.Options {
	if !o.Text.Enabled {
		o.Text.Text = ""
	}
	if !o.Logo.Enabled {
		o.Logo.Path = ""
	}
	for i, logo := range o.AdditionalLogos {
		if !logo.Enabled {
			o.AdditionalLogos[i] = imaging.LogoMark{}
		}
	}
	return withContentWatermarks(o)
}

func (m *Model) RefreshPlan() {
	m.Plan, m.PlanErr = files.Plan{}, nil
	m.schedulePlan(false)
}

// Settings edits can leave the last preview on screen until the latest
// revision is ready. Source and destination changes invalidate that snapshot.
func (m *Model) schedulePlan(immediate bool) {
	if m.Working || len(m.Sources) == 0 {
		m.planner.Cancel()
		m.Planning = false
		return
	}
	m.Planning = true
	req := planRequest{slices.Clone(m.Sources), m.Output, m.Naming, m.Image.Format.Extension(), time.Now()}
	if immediate {
		m.planner.SubmitNow(req)
	} else {
		m.planner.Submit(req)
	}
}

func (m *Model) ShowThumbnails(paths []string) {
	if m.Working || slices.Equal(m.visible, paths) {
		return
	}
	m.visible = slices.Clone(paths)
	var work []string
	for _, p := range paths {
		if _, ok := m.thumbs[p]; !ok {
			work = append(work, p)
		}
	}
	if len(work) == 0 {
		m.thumbnailer.Cancel()
	} else {
		m.thumbnailer.Submit(work)
	}
	m.scheduleWarmup()
}

func (m *Model) Thumbnail(path string) Thumb { return m.thumbs[path] }

func (m *Model) CurrentEntry() (files.Entry, bool) {
	if m.Planning || m.PlanErr != nil {
		return files.Entry{}, false
	}
	for _, e := range m.Plan.Entries {
		if e.Source.Path == m.Selected && (e.Source.Part == 0 || e.Source.Part == m.PreviewPart) {
			return e, true
		}
	}
	return files.Entry{}, false
}

func (m *Model) SetPreferences(p config.Preferences) {
	m.Preferences = p
	m.ScopeRevision++
	m.AppliedPreset = m.presetID(m.focusSettings())
	if m.configReadOnly {
		return
	}
	m.PrefsPending, m.configDirty = true, true
	m.saver.Submit(p.Clone())
}

func (m *Model) Start(retry []files.Source) error {
	if m.Working || m.Scanning || m.LogoPending || m.CertificationImporting || m.Collage.Exporting {
		return l10n.New("error.waitForOperation")
	}
	src := m.Sources
	job := Job{Image: m.Image, Naming: m.Naming, Output: m.Output, Workers: m.Preferences.Workers, Date: time.Now()}
	if p, ok := m.Preferences.Preset(m.AppliedPreset); ok {
		job.Preset = p
	}
	if retry != nil {
		src = retry
		job.Image, job.Naming, job.Date, job.Preset = m.failureJob.Image, m.failureJob.Naming, m.failureJob.Date, m.failureJob.Preset
	}
	if len(src) == 0 {
		return l10n.New("error.noImagesToProcess")
	}
	var err error
	src, err = files.PrepareSources(m.ctx, src)
	if err != nil {
		return err
	}
	// Every entry carries the immutable settings it will render, including on retry.
	checked := map[*files.PhotoSettings]bool{}
	job.Mixed = false
	first := m.PhotoSettings(src[0])
	for _, source := range src {
		if checked[source.Settings] {
			continue
		}
		checked[source.Settings] = true
		if p := m.PhotoSettings(source); p != first {
			job.Mixed = true
		}
		opt := job.Image
		if source.Settings != nil {
			opt = source.Settings.Image
		}
		if err := opt.Validate(); err != nil {
			return l10n.WrapWithArgs("error.photoSettings", err, l10n.Args{"file": filepath.Base(source.Path)})
		}
	}
	if m.Output.Directory == "" {
		return l10n.New("error.chooseOutputFolder")
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.jobCancel = cancel
	lastJob := m.Job
	m.Working, m.Cancelling, m.Job = true, false, job
	m.Progress = batch.Progress{Total: len(src), Remaining: len(src), Started: time.Now()}
	m.clearPreview()
	m.preview.cache = baseCache{}
	m.thumbnailer.Cancel()
	m.RefreshPlan()
	m.Rendering = false
	m.wg.Go(func() {
		defer cancel()
		p, err := files.BuildPlan(ctx, src, job.Output, job.Naming, job.Image.Format.Extension(), job.Date)
		var r batch.Report
		if err == nil {
			r, err = batch.Run(ctx, p, job.Image, job.Workers, func(pr batch.Progress) {
				m.onUI(func() { m.Progress = pr })
			})
		}
		m.onUI(func() {
			m.Working, m.Cancelling = false, false
			m.jobCancel = nil
			if err != nil {
				if errors.Is(err, context.Canceled) {
					now := time.Now()
					m.Progress.Cancelled, m.Progress.Remaining, m.Progress.Finished = len(src), 0, now
					m.Report = batch.Report{Progress: m.Progress}
				} else {
					m.Error = l10n.Wrap("error.cannotStart", err)
					m.Progress, m.Job = m.Report.Progress, lastJob
				}
			} else {
				m.Report, m.Progress = r, r.Progress
				m.Failures = reconcileFailures(m.Failures, r, retry != nil)
				if retry == nil {
					m.failureJob = job
				}
				if !slices.Contains(m.knownOutputs, r.Directory) {
					m.knownOutputs = append(m.knownOutputs, r.Directory)
				}
			}
			m.visible = nil
			m.requestPreview()
			m.RefreshPlan()
		})
	})
	return nil
}

func (m *Model) Cancel() {
	if m.jobCancel != nil {
		m.Cancelling = true
		m.jobCancel()
	}
}
