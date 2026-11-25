// Package ui implements the MochiPic desktop workspace in native MyGo.
package ui

import (
	"log"
	"path/filepath"
	"strings"

	"imageflow/internal/app"
	"imageflow/internal/batch"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/winupdate"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

const Version = "0.0.26"

type page int

type viewKey struct {
	panel string
	page  page
}

const (
	pictures page = iota
	watermark
	resize
	format
	rename
	output
	tasks
	failures
	presets
	settings
	collage
	importHome
	certificationPage
	captionPage
)

type desktop struct {
	geometry                                           *windowGeometry
	outputTree                                         *treeFolder
	treeSource                                         *files.Entry
	collapsedFolders                                   map[string]bool
	errorFocus                                         string
	editingPreset                                      string
	model                                              *app.Model
	win                                                *mygo.Window
	historyMenuUndo, historyMenuRedo                   *mygo.MenuItem
	page                                               page
	hideSidebar, hideInspector                         bool
	importMenu                                         bool
	importAttempted, menuHasImages                     bool
	list                                               bool
	watermarkTab                                       int
	logoSelection                                      string
	seenLogoImport                                     uint64
	logoPage                                           int
	resizeTab                                          int
	crop                                               cropDraft
	collage                                            collageView
	seriesPrompt                                       string
	dialog                                             bool
	creditsOpen                                        bool
	updates                                            *updateControls
	quitWarning                                        bool
	mixed                                              mixedInspector
	strip                                              filmstripState
	errorSelected                                      map[string]bool
	skippedFailures                                    map[string]bool
	listQuery, listSort                                string
	listPage, thumbPage, namePage, treePage, errorPage int
	listVisible                                        int
	listAnchor                                         string
	lastSelected                                       string
	chosenPreset, presetName                           string
	presetModal                                        bool
	presetReplace                                      bool
	zoom, fit                                          float32
	pan                                                ui.ScrollState
	centerWidth                                        float32
}

func Run() error {
	d := &desktop{page: importHome, errorSelected: map[string]bool{}, listSort: "order"}
	d.model = app.New(mygo.RunOnMain, d.redraw)
	mygo.App.SetName(d.t("app.name"))
	mygo.App.SetVersion(Version)
	mygo.App.SetLocale(d.model.Preferences.Language)
	d.configureUpdates()
	mygo.App.OnLocaleChanged(func(locale string) {
		name := l10n.T(locale, "app.name")
		mygo.App.SetName(name)
		if d.win != nil {
			d.win.SetTitle(name)
			d.installMenu()
		}
	})
	mygo.App.OnOpenFile(func(path string) { d.importPaths([]string{path}) })
	mygo.App.WhenReady(func() {
		mygo.Theme.SetSource(mygo.ThemeSource(d.model.Preferences.Theme))
		d.openMainWindow()
		d.installMenu()
		d.win.OnFileDrop(func(e *mygo.FileDropEvent) { d.importPaths(e.Paths) })
		visualQA(d)
	})
	err := mygo.App.Run()
	if e := d.model.Close(); e != nil {
		log.Printf("saving preferences: %v", e)
	}
	if err == nil && d.updates.restart {
		return winupdate.ErrRestart
	}
	return err
}

func (d *desktop) redraw() {
	d.syncPage()
	if d.win != nil {
		d.geometry.setCompact(d.page == importHome)
		if d.menuHasImages != (len(d.model.Sources) > 0) {
			d.installMenu()
		}
		d.win.Invalidate()
	}
}

func (d *desktop) emptyBatch() bool { return len(d.model.Sources) == 0 && !d.model.Working }

func (d *desktop) syncPage() {
	switch {
	case d.model.Working:
		d.page = tasks
	case d.emptyBatch():
		if d.page != settings {
			d.page = importHome
		}
	case d.page == importHome && d.importAttempted && d.model.Selected != "" && d.model.PreviewErr == nil && d.model.BeforeFrame().Image == nil:
		d.model.ShowThumbnails([]string{d.model.Selected})
	case d.page == importHome || d.importAttempted:
		d.importAttempted = false
		d.page = pictures
	}
}

func (d *desktop) installMenu() {
	d.menuHasImages = len(d.model.Sources) > 0
	empty := !d.menuHasImages
	call := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { fn(); d.redraw() }
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: d.t("app.name"), Submenu: []*mygo.MenuItem{
			{Label: d.t("menu.settings"), Accelerator: "CmdOrCtrl+,", Click: call(func() { d.navigate(settings) })},
			d.updateMenuItem(),
			mygo.Separator(), {Role: mygo.RoleHide}, {Role: mygo.RoleQuit},
		}},
		{Label: d.t("menu.file"), Submenu: []*mygo.MenuItem{
			{Label: d.t("menu.newBatch"), Accelerator: "CmdOrCtrl+N", Click: call(d.newBatch)},
			{Label: d.t("import.imagesMenu"), Accelerator: "CmdOrCtrl+O", Click: call(func() { d.pickImport(false) })},
			{Label: d.t("import.foldersMenu"), Accelerator: "CmdOrCtrl+Shift+O", Click: call(func() { d.pickImport(true) })},
			{Label: d.t("menu.chooseOutputFolder"), Disabled: empty, Click: call(func() { d.pickDirectory(false) })},
			mygo.Separator(),
			{Label: d.t("task.start"), Disabled: empty, Click: call(func() { d.start(nil) })}, {Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu, Label: d.t("menu.edit"), Submenu: []*mygo.MenuItem{
			{ID: "history-undo", Role: mygo.RoleUndo, Label: d.t("mygo.menuUndo")},
			{ID: "history-redo", Role: mygo.RoleRedo, Label: d.t("mygo.menuRedo")},
			mygo.Separator(), {Role: mygo.RoleCut}, {Role: mygo.RoleCopy}, {Role: mygo.RolePaste},
			{Role: mygo.RolePasteAndMatchStyle}, {Role: mygo.RoleDelete}, {Role: mygo.RoleSelectAll},
			mygo.Separator(), {Label: d.t("mygo.menuSpeech"), Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleStartSpeaking}, {Role: mygo.RoleStopSpeaking},
			}},
		}},
		{Label: d.t("menu.view"), Submenu: []*mygo.MenuItem{
			{Label: d.t("view.fitWindow"), Accelerator: "CmdOrCtrl+0", Disabled: empty, Click: call(func() { d.setZoom(0) })},
			{Label: d.t("view.actualPreviewPixels"), Accelerator: "CmdOrCtrl+1", Disabled: empty, Click: call(func() { d.setZoom(-1) })},
			{Label: d.t("view.toggleSidebar"), Disabled: empty, Click: call(func() { d.hideSidebar = !d.hideSidebar })},
			{Label: d.t("view.toggleInspector"), Disabled: empty, Click: call(func() { d.hideInspector = !d.hideInspector })},
		}},
		{Role: mygo.RoleWindowMenu, Label: d.t("menu.window")},
	})
	d.historyMenuUndo, d.historyMenuRedo = menu.ItemByID("history-undo"), menu.ItemByID("history-redo")
	mygo.App.SetMenu(menu)
}

func (d *desktop) navigate(p page) {
	if d.model.Collage.Exporting || d.model.Working && p != tasks || d.emptyBatch() && p != importHome && p != settings {
		return
	}
	previous := d.page
	if p != resize {
		d.crop = cropDraft{}
	}
	d.page = p
	if p != previous && !(d.viewerPage(previous) && d.viewerPage(p)) {
		d.model.ShowThumbnails(nil)
	}
	switch p {
	case watermark, captionPage:
		d.model.SetMode(app.After)
	case certificationPage:
		d.model.SetMode(app.After)
		d.model.LoadCertifications()
	case resize:
		if d.resizeTab == 0 && previous != resize {
			d.beginCrop()
		} else if !d.cropping() {
			d.model.SetMode(app.After)
		}
	case format:
		d.model.SetMode(app.Encoded)
	default:
		d.model.SetMode(app.Before)
	}
	d.redraw()
}

func (d *desktop) viewerPage(p page) bool {
	return p == watermark || p == certificationPage || p == captionPage || p == resize || p == format || p == pictures && !d.list
}

func (d *desktop) canvasKey() viewKey {
	if d.viewerPage(d.page) {
		return viewKey{"viewer", pictures}
	}
	return viewKey{"main", d.page}
}

func (d *desktop) importPaths(paths []string) {
	if len(paths) == 0 || d.model.Working || d.model.Collage.Exporting {
		return
	}
	d.list = false
	if d.emptyBatch() {
		d.importAttempted = true
		d.navigate(importHome)
	} else {
		d.navigate(pictures)
	}
	d.model.SetMode(app.Before)
	d.model.Import(paths)
	d.redraw()
}

func (d *desktop) pick(opts mygo.OpenDialogOptions, consume func([]string)) {
	if d.dialog || d.win == nil || d.model.Working || d.model.Collage.Exporting {
		return
	}
	d.dialog = true
	opts.Parent = d.win
	go func() {
		paths, err := mygo.Dialog.Open(opts)
		mygo.RunOnMain(func() {
			d.dialog = false
			if err != nil {
				d.model.Error = err
			} else if len(paths) > 0 {
				consume(paths)
			}
			d.redraw()
		})
	}()
}
func (d *desktop) newBatch() {
	if d.model.Working || d.model.Scanning || d.model.LogoPending || d.model.CertificationImporting || d.model.Collage.Exporting {
		return
	}
	d.model.NewBatch()
	d.logoSelection, d.logoPage = "", 0
	d.importAttempted, d.importMenu = false, false
	d.seriesPrompt = ""
	d.resizeTab, d.crop = 0, cropDraft{}
	d.strip = filmstripState{}
	clear(d.errorSelected)
	clear(d.skippedFailures)
	d.listAnchor, d.errorFocus = "", ""
	d.outputTree, d.treeSource, d.collapsedFolders = nil, nil, nil
	d.list, d.thumbPage, d.listPage, d.namePage, d.treePage = false, 0, 0, 0, 0
	d.navigate(importHome)
}
func (d *desktop) pickImport(folder bool) {
	opts := mygo.OpenDialogOptions{Directory: folder, Multiple: true, Title: d.t("import.images"), ButtonLabel: d.t("import.action")}
	if folder {
		opts.Title = d.t("import.folders")
	} else {
		opts.Filters = []mygo.FileFilter{{Name: d.t("nav.images"), Extensions: imaging.InputExtensions()}}
	}
	d.pick(opts, d.importPaths)
}
func (d *desktop) pickDirectory(defaultDir bool) {
	if !defaultDir && d.emptyBatch() {
		return
	}
	dir := d.model.Output.Directory
	if defaultDir {
		dir = d.model.Preferences.Output.Directory
	}
	d.pick(mygo.OpenDialogOptions{Title: d.t("output.chooseOutputFolder"), ButtonLabel: d.t("common.choose"), Directory: true, CreateDirectories: true, DefaultPath: dir}, func(paths []string) {
		if defaultDir {
			p := d.model.Preferences
			p.Output.Directory = paths[0]
			d.model.SetPreferences(p)
		} else {
			o := d.model.Output
			o.Directory = paths[0]
			d.model.SetOutput(o)
		}
	})
}

func (d *desktop) start(retry []files.Source) {
	if d.cropping() || d.page == collage || d.model.Working || d.model.Scanning || d.model.LogoPending || d.model.CertificationImporting || len(d.model.Sources) == 0 && retry == nil {
		return
	}
	if retry == nil {
		if source, err := d.model.FirstSeriesIssue(); err != nil {
			d.seriesPrompt, d.list = source.Path, false
			d.model.Select(source.Path)
			d.navigate(pictures)
			return
		}
	}
	if d.model.Output.Directory == "" {
		d.navigate(output)
		d.pickDirectory(false)
		return
	}
	if err := d.model.Start(retry); err != nil {
		d.model.Error = err
		d.redraw()
		return
	}
	if retry == nil {
		clear(d.skippedFailures)
	}
	d.errorSelected = map[string]bool{}
	d.errorPage = 0
	d.navigate(tasks)
}

func (d *desktop) hasInspector() bool {
	switch d.page {
	case pictures:
		return !d.list && d.model.Selected != ""
	case tasks:
		return d.model.Working
	case settings:
		return false
	default:
		return true
	}
}

func (d *desktop) view(c *ui.Context) {
	theme.Apply(c)
	tk := theme.TokensOf(c)
	m := d.model
	d.syncPage()
	d.syncCropDraft()
	if d.page == watermark {
		d.syncLogoSelection()
	}
	if d.page != pictures || d.list {
		d.strip.drag = thumbnailDrag{}
	}
	if d.strip.drag.path != "" && c.Shortcut(0, ui.KeyEscape) {
		d.strip.drag.cancelled = true
	}
	d.historyShortcuts(c)
	if d.cropping() && c.Shortcut(0, ui.KeyEscape) {
		d.endCrop(false)
	}
	if !c.FocusTakesText() && d.page == pictures && d.list {
		if c.Shortcut(ui.Cmd, ui.KeyA) {
			d.selectLibraryItems(d.libraryItems(), true)
		}
		if (c.Shortcut(0, ui.KeyBackspace) || c.Shortcut(0, ui.KeyDelete)) && len(d.model.EditSelection) > 0 {
			d.removeSelected()
		}
	}
	if m.Selected != d.lastSelected {
		d.zoom, d.fit = 0, 0
		d.pan = ui.ScrollState{}
		d.lastSelected = m.Selected
	}
	empty := d.emptyBatch() || d.page == importHome
	root := ui.Column(c).Fill().Background(choose(empty, tk.BgContainer, tk.BgLayout)).FontSize(13)
	dragging := false
	if !m.Working && !m.Collage.Exporting && d.page != collage && !d.creditsOpen {
		dragging = empty && root.FileDragOver()
		if paths := root.DroppedFiles(); len(paths) > 0 {
			d.importPaths(paths)
		}
	}
	root.Children(func() {
		d.titlebar(c, !empty)
		if empty {
			if d.page == settings {
				d.importSettings(c.Key("import-settings"))
			} else {
				d.importWelcome(c.Key("import-home"), dragging)
			}
		} else {
			d.workspace(c.Key("workspace"))
		}
		d.creditsModal(c.Key("credits-modal"))
		if m.Error != nil {
			open := true
			feedback.Modal(c.Key("error-modal"), &open, feedback.ModalProps{Title: tr(c, "common.operationFailed"), OkText: tr(c, "common.ok"), NoFooter: true, Width: 460}, func() {
				ui.Column(c).Gap(16).Children(func() {
					labelWrap(c, errText(c, m.Error))
					if button(c, tr(c, "common.ok"), nil, action.Primary, false) {
						open = false
					}
				})
			})
			if !open {
				m.Error = nil
			}
		}
	})
	d.syncHistoryMenu(c)
}

func (d *desktop) workspace(c *ui.Context) {
	tk := theme.TokensOf(c)
	w, _ := c.Size()
	sideW, rightW := float32(152), float32(324)
	if w < 1190 {
		sideW, rightW = 136, 292
	}
	if d.page == certificationPage {
		rightW = 330
	}
	if c.Locale() != "" && c.Locale() != l10n.Default {
		sideW += 30
	}
	if d.hideSidebar {
		sideW = 0
	}
	if d.hideInspector || !d.hasInspector() {
		rightW = 0
	}
	d.centerWidth = max(380, w-sideW-rightW-60)
	ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Padding(0, 8, 8, 0).Children(func() {
		if sideW > 0 {
			d.sidebar(c, sideW)
		}
		board := ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).
			Background(tk.BgContainer).Radius(tk.BorderRadiusLG).Border(1, tk.BorderSecondary).Clip()
		if sideW == 0 {
			board.Margin(0, 0, 0, 8)
		}
		board.Children(func() {
			d.toolbar(c.Key(viewKey{"toolbar", d.page}))
			line(c)
			ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				d.mainContent(c.Key(d.canvasKey()))
				if rightW > 0 {
					verticalLine(c)
					d.inspector(c.Key(viewKey{"inspector", d.page}), rightW)
				}
			})
			if d.page != settings {
				line(c)
				footer := c.Key(viewKey{"footer", d.page})
				if d.page == pictures && !d.list {
					d.pictureStatusbar(footer, rightW)
				} else {
					d.statusbar(footer)
				}
			}
		})
	})
}

func (d *desktop) titlebar(c *ui.Context, workspace bool) {
	tk := theme.TokensOf(c)
	inset := max(12, c.TitleBar().Left+6)
	side := max(inset+106, c.TitleBar().Right+6)
	ui.Row(c).Height(42).Shrink(0).AlignItems(ui.Center).Background(tk.BgLayout).DragWindow().Children(func() {
		ui.Row(c).Width(side).Shrink(0).Padding(0, 0, 0, inset).AlignItems(ui.Center).Gap(5).Children(func() {
			if workspace {
				if smallIcon(c.Key("sidebar"), lucide.PanelLeft(), tr(c, "view.toggleSidebar"), d.hideSidebar, false) {
					d.hideSidebar = !d.hideSidebar
				}
				ui.Box(c).Size(1, 16).Shrink(0).Background(tk.BorderSecondary)
			} else {
				ui.Box(c).Width(30).Shrink(0)
			}
			if workspace || d.model.UndoAction() != "" || d.model.RedoAction() != "" {
				d.historyButtons(c.Key("history"))
			}
		})
		ui.Box(c).Grow(1).MinWidth(0).Center().Children(func() { strong(c, tr(c, "app.name")).SingleLine() })
		ui.Box(c).Width(side).Shrink(0)
	})
}

func (d *desktop) sidebar(c *ui.Context, w float32) {
	ui.Column(c).Width(w).Shrink(0).Padding(10, 10, 5, 10).Children(func() {
		d.importTrigger(c.Key("import"))
		ui.Column(c).Margin(20, 0, 0, 0).Gap(3).Children(func() {
			d.navItem(c, pictures, tr(c, "nav.images"), lucide.Images())
			d.navItem(c, watermark, tr(c, "nav.watermark"), lucide.Stamp())
			d.navItem(c, certificationPage, tr(c, "nav.certification"), lucide.BadgeCheck())
			d.navItem(c, captionPage, tr(c, "nav.caption"), lucide.PanelBottom())
			d.navItem(c, resize, tr(c, "nav.resize"), lucide.Scaling())
			d.navItem(c, format, tr(c, "nav.format"), lucide.FileImage())
			d.navItem(c, rename, tr(c, "nav.rename"), lucide.TextCursorInput())
			d.navItem(c, output, tr(c, "nav.output"), lucide.FolderOutput())
			d.navItem(c, tasks, tr(c, "nav.results"), lucide.ListChecks())
		})
		ui.Box(c).Grow(1)
		ui.Column(c).Gap(3).Children(func() {
			d.navItem(c, presets, tr(c, "nav.presets"), lucide.Layers())
			d.navItem(c, settings, tr(c, "nav.settings"), lucide.Settings())
		})
	})
}

func (d *desktop) importTrigger(c *ui.Context) {
	tk, m := theme.TokensOf(c), d.model
	if m.Working || m.Collage.Exporting {
		d.importMenu = false
	}
	p := overlay.DropdownMenuProps{Items: []overlay.MenuItem{
		{Key: "images", Label: tr(c, "import.imagesMenu"), Icon: lucide.ImagePlus(), Hotkey: "mod+o"},
		{Key: "folders", Label: tr(c, "import.foldersMenu"), Icon: lucide.FolderInput(), Hotkey: "mod+shift+o"},
		{Divider: true},
		{Key: "recursive", Label: tr(c, "import.recursive"), Icon: choose(m.Preferences.Recursive, lucide.Check(), nil), Disabled: m.Scanning},
	}}
	switch overlay.DropdownMenu(c, &d.importMenu, p, func(b ui.Element) {
		action.StyleButton(c, b.Disabled(m.Working), "", action.ButtonProps{Variant: action.Filled})
		b.Label(tr(c, "import.action")).Height(36).Radius(tk.BorderRadius).AlignSelf(ui.Stretch).PaddingX(10).AlignItems(ui.Center)
		b.Children(func() {
			icon(c, lucide.Plus(), 15, tk.TextSecondary)
			strong(c, tr(c, "import.action"))
			ui.Box(c).Grow(1)
			icon(c, lucide.ChevronDown(), 13, tk.TextSecondary)
		})
	}) {
	case "images":
		d.pickImport(false)
	case "folders":
		d.pickImport(true)
	case "recursive":
		d.setRecursive(!m.Preferences.Recursive)
	}
}

func (d *desktop) setRecursive(recursive bool) {
	if d.model.Scanning || recursive == d.model.Preferences.Recursive {
		return
	}
	p := d.model.Preferences
	p.Recursive = recursive
	d.model.SetPreferences(p)
}

func (d *desktop) navItem(c *ui.Context, p page, title string, svg *ui.SVG) {
	tk := theme.TokensOf(c)
	active := p == d.page || p == tasks && d.page == failures || p == pictures && d.page == collage
	btn := ui.ButtonBase(c.Key(p)).Height(37).Radius(tk.BorderRadius).Label(title).Tooltip(title).
		Disabled(d.model.Working && p != tasks || d.model.Collage.Exporting).AlignItems(ui.Center).Justify(ui.Start).PaddingX(10).Gap(9)
	if active || btn.Hovered() {
		btn.Background(tk.FillSecondary)
	}
	fg := choose(active, tk.Text, tk.TextSecondary)
	btn.Children(func() {
		icon(c, svg, 16, fg)
		ui.Text(c, title).TextColor(fg).FontWeight(choose(active, 600, 400)).SingleLine()
	})
	if btn.Clicked() {
		d.navigate(p)
	}
}

func (d *desktop) toolbar(c *ui.Context) {
	m := d.model
	keys := [...]string{pictures: "nav.images", watermark: "nav.watermark", resize: "nav.resize", format: "nav.format",
		rename: "nav.rename", output: "nav.output", tasks: "nav.results", failures: "nav.errors", presets: "nav.presets",
		settings: "nav.settings", collage: "nav.collage", certificationPage: "nav.certification", captionPage: "nav.caption"}
	title := tr(c, keys[d.page])
	ui.Row(c).Height(52).Shrink(0).PaddingX(18).AlignItems(ui.Center).Gap(8).Children(func() {
		if d.page == collage && smallIcon(c, lucide.ArrowLeft(), tr(c, "collage.back"), false, m.Collage.Exporting) {
			d.leaveCollage()
		}
		if d.page == failures && smallIcon(c, lucide.ArrowLeft(), tr(c, "view.backToResults"), false, false) {
			d.navigate(tasks)
		}
		strong(c, title).FontSize(16)
		if d.page == pictures && len(m.Sources) > 0 {
			d.libraryMode(c)
		}
		if d.page == rename && smallIcon(c, lucide.Copy(), tr(c, "rename.copyPreview"), false, len(m.Plan.Entries) == 0 || m.Planning || m.PlanErr != nil) {
			d.copyPlan(c)
		}
		ui.Box(c).Grow(1).MinWidth(0)
		switch d.page {
		case collage:
			d.collageToolbar(c)
		case resize:
			if d.cropping() {
				if button(c, tr(c, "common.cancel"), nil, action.Text, false) {
					d.endCrop(false)
				}
				if button(c, tr(c, "crop.done"), lucide.Check(), action.Primary, !d.cropReady()) {
					d.endCrop(true)
				}
				return
			}
			d.processingToolbar(c)
		case tasks:
			if m.Working && dangerButton(c, choose(m.Cancelling, tr(c, "task.cancelling"), tr(c, "task.cancel")), lucide.X(), m.Cancelling) {
				m.Cancel()
			}
		case failures:
			if button(c, tr(c, "failure.retryAll"), lucide.RotateCcw(), action.Primary, d.retryableCount() == 0) {
				d.retrySelected(true)
			}
		case presets:
			if button(c, tr(c, "preset.create"), lucide.Plus(), action.Default, len(m.Preferences.Presets) >= maxPresets) {
				d.editingPreset, d.presetName, d.presetModal = "", "", true
			}
			if d.hideInspector {
				d.inspectorToggle(c)
			}
		case settings:
		default:
			d.processingToolbar(c)
		}
		// These pages keep their toolbars quiet until the hidden panel needs a way back.
		if d.hasInspector() && d.hideInspector && (d.page == output || d.page == tasks || d.page == failures) {
			d.inspectorToggle(c)
		}
	})
}

func (d *desktop) processingToolbar(c *ui.Context) {
	m := d.model
	if d.page != output && (d.page != pictures || len(m.Sources) > 0) {
		d.presetSelect(c)
		if smallIcon(c, lucide.FolderOpen(), tr(c, "output.chooseOutputFolder"), false, false) {
			d.pickDirectory(false)
		}
		if d.hasInspector() {
			d.inspectorToggle(c)
		}
	}
	if button(c, trf(c, "scope.exportAll", l10n.Args{"count": len(m.Sources)}), lucide.FolderOutput(), action.Primary, m.Working || m.Scanning || m.LogoPending || m.CertificationImporting || len(m.Sources) == 0) {
		d.start(nil)
	}
}

func (d *desktop) inspectorToggle(c *ui.Context) {
	if smallIcon(c, lucide.PanelRight(), tr(c, "view.toggleInspector"), d.hideInspector, false) {
		d.hideInspector = !d.hideInspector
	}
}

func (d *desktop) presetSelect(c *ui.Context) {
	m := d.model
	current := m.AppliedPreset
	if m.SelectionHasMixedPresets() {
		current = ""
	} else if current == "" {
		current = "custom"
	}
	v := current
	opts := []field.SelectOption{{Value: "custom", Label: tr(c, "preset.custom"), Icon: lucide.Layers()}}
	for _, p := range m.Preferences.Presets {
		opts = append(opts, field.SelectOption{Value: p.ID, Label: presetDisplayName(c, p), Icon: lucide.Layers()})
	}
	width := float32(138)
	if c.Locale() != "" && c.Locale() != l10n.Default {
		width = 186
	}
	field.Select(c.Key("current-preset"), &v, field.SelectProps{Options: opts, Placeholder: tr(c, "scope.mixed"), Variant: field.InputBorderless, Size: theme.Small}).Width(width).Shrink(0).Label(tr(c, "preset.current"))
	if v != current {
		if p, ok := m.Preferences.Preset(v); ok {
			m.ApplyPreset(p)
		} else {
			m.ClearSelectedPreset()
		}
	}
}

const statusBarHeight = 33

func (d *desktop) statusbar(c *ui.Context) {
	m, tk := d.model, theme.TokensOf(c)
	ui.Row(c).Height(statusBarHeight).Shrink(0).PaddingX(14).Gap(8).AlignItems(ui.Center).Children(func() {
		safety := true
		switch d.page {
		case pictures, resize, certificationPage, captionPage, watermark, format:
			if d.page == pictures {
				d.imageTotal(c)
				muted(c, trf(c, "image.selectedCount", l10n.Args{"count": len(m.EditSelection)}))
			} else {
				icon(c, lucide.CircleCheck(), 13, tk.TextSecondary)
				muted(c, tr(c, "scope.saved"))
			}
			d.scanStatus(c)
			if d.page == pictures && d.list {
				safety = false
				ui.Box(c).Grow(1)
				pager(c, d.listVisible, listPerPage, &d.listPage)
			}
		case rename:
			safety = false
			if m.Output.Directory == "" && len(m.Sources) > 0 {
				muted(c, tr(c, "rename.chooseFolderHint")).MinWidth(0).Shrink(1)
			}
			ui.Box(c).Grow(1)
			pager(c, len(m.Plan.Entries), namePerPage, &d.namePage)
		case collage:
			d.collageStatus(c)
		case output:
			muted(c, trf(c, "image.count", l10n.Args{"count": len(m.Sources)}))
			if m.Planning {
				muted(c, tr(c, "output.planningStatus"))
			}
		case tasks:
			if m.Progress.Total > 0 {
				icon(c, lucide.Clock(), 13, tk.TextSecondary)
				muted(c, trf(c, "task.elapsed", l10n.Args{"duration": elapsed(m.Progress)}))
			}
		case failures:
			muted(c, trf(c, "failure.selectionCount", l10n.Args{"failed": len(m.Failures), "selected": len(d.errorSelected)}))
			if n := len(d.skippedFailures); n > 0 {
				muted(c, trf(c, "failure.excludedCount", l10n.Args{"count": n}))
			}
			if len(m.Failures) > errorPerPage {
				pager(c, len(m.Failures), errorPerPage, &d.errorPage)
			}
		case presets:
			safety = false
			muted(c, trf(c, "preset.count", l10n.Args{"count": len(m.Preferences.Presets)}))
		}
		if safety {
			ui.Box(c).Grow(1)
			originalsProtected(c)
		}
	})
}

func (d *desktop) imageTotal(c *ui.Context) {
	m := d.model
	if m.Scanning {
		muted(c, tr(c, "import.scanning")).MinWidth(0).Shrink(1)
		return
	}
	icon(c, lucide.Images(), 13, theme.TokensOf(c).TextSecondary)
	muted(c, trf(c, "image.count", l10n.Args{"count": len(m.Sources)})).MinWidth(0).Shrink(1)
}

func originalsProtected(c *ui.Context) {
	icon(c, lucide.ShieldCheck(), 13, theme.TokensOf(c).TextSecondary)
	muted(c, tr(c, "output.originalsProtected")).MinWidth(0).Shrink(1)
}

func (d *desktop) scanStatus(c *ui.Context) {
	r := d.model.Scan
	if r.Duplicates+r.Ignored+r.Errors == 0 && !r.Limited {
		return
	}
	notice := scanNotice(c, r)
	if r.Errors > 0 {
		count := trf(c, "import.unreadableCount", l10n.Args{"count": r.Errors})
		if styledButtonSize(c, count, lucide.CircleAlert(), action.Text, theme.Small, d.dialog, false).Tooltip(notice).Clicked() {
			d.showScanIssues(c)
		}
	} else {
		ui.Box(c).Size(20, 20).Center().Tooltip(notice).Label(notice).Children(func() {
			icon(c, lucide.Info(), 13, theme.TokensOf(c).TextSecondary)
		})
	}
}

func scanNotice(c *ui.Context, report batch.ScanReport) string {
	notice := trf(c, "import.summary", l10n.Args{"added": len(report.Added), "duplicates": report.Duplicates, "ignored": report.Ignored})
	if report.Errors > 0 {
		notice += " · " + trf(c, "import.unreadableCount", l10n.Args{"count": report.Errors})
	}
	if report.SkippedOutputFolder {
		notice += " · " + tr(c, "import.outputFolderSkipped")
	}
	if report.Limited {
		notice += " · " + trf(c, "import.limitReached", l10n.Args{"limit": batch.MaxFiles})
	}
	return notice
}

func (d *desktop) showScanIssues(c *ui.Context) {
	report := d.model.Scan
	if d.dialog || len(report.Issues) == 0 {
		return
	}
	issues := report.Issues[:min(8, len(report.Issues))]
	var rows []string
	for _, item := range issues {
		rows = append(rows, filepath.Base(item.Path)+": "+errText(c, item.Err))
	}
	if more := report.Errors - len(issues); more > 0 {
		rows = append(rows, trf(c, "import.moreIssues", l10n.Args{"count": more}))
	}
	opts := mygo.MessageOptions{Parent: d.win, Type: mygo.MessageWarning,
		Message: trf(c, "import.unreadableCount", l10n.Args{"count": report.Errors}), Detail: strings.Join(rows, "\n")}
	d.dialog = true
	go func() {
		_, _ = mygo.Dialog.Message(opts)
		mygo.RunOnMain(func() {
			d.dialog = false
			d.redraw()
		})
	}()
}

func (d *desktop) mainContent(c *ui.Context) {
	box := ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0)
	box.Children(func() {
		switch d.page {
		case pictures:
			if d.list {
				d.library(c)
			} else {
				d.viewer(c)
			}
		case watermark, certificationPage, captionPage, resize, format:
			d.viewer(c)
		case rename:
			d.planPage(c, false)
		case output:
			d.planPage(c, true)
		case tasks:
			d.taskPage(c)
		case failures:
			d.errorPageView(c)
		case presets:
			d.presetsPage(c)
		case settings:
			d.settingsPage(c)
		case collage:
			d.collageViewer(c)
		}
	})
}

func (d *desktop) inspector(c *ui.Context, width float32) {
	ui.Column(c).Width(width).Shrink(0).MinHeight(0).Children(func() {
		if d.editPage() || d.page == output || d.page == presets {
			d.scopeHeader(c.Key("edit-range"))
		}
		switch d.page {
		case pictures:
			d.imageInspector(c)
		case watermark:
			d.watermarkInspector(c)
		case certificationPage:
			d.certificationInspector(c)
		case captionPage:
			d.captionInspector(c)
		case resize:
			d.dimensionsInspector(c)
		case collage:
			d.collageInspector(c)
		case format:
			d.formatInspector(c)
		case rename:
			d.renameInspector(c)
		case output:
			d.outputInspector(c)
		case tasks:
			d.taskInspector(c)
		case failures:
			d.errorInspector(c)
		case presets:
			d.presetInspector(c)
		}
	})
}

func baseName(c *ui.Context, path string) string {
	if path == "" {
		return tr(c, "common.notChosen")
	}
	return filepath.Base(path)
}

func tag(c *ui.Context, s string, status display.TagColor) {
	display.Tag(c, s, display.TagProps{Size: theme.Small, Color: status})
}
