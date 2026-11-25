//go:build imageflowqa

package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"time"

	"imageflow/internal/app"
	"imageflow/internal/caption"
	"imageflow/internal/certification"
	"imageflow/internal/config"
	"imageflow/internal/crop"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// visualQA takes screenshots from the running native Window. The dev script
// supplies only local fixtures and a temporary preferences directory.
func visualQA(d *desktop) {
	fixtures := os.Getenv("IMAGEFLOW_VISUAL_FIXTURES")
	target := os.Getenv("IMAGEFLOW_VISUAL_CAPTURE")
	if fixtures == "" || target == "" {
		return
	}
	go func() {
		err := captureSequence(d, fixtures, target)
		if err != nil {
			log.Printf("visual QA: %v", err)
			os.Exit(1)
		}
		log.Print("visual QA: native window screenshots complete")
		mygo.App.Quit()
	}()
}

func captureComposition(d *desktop, fixtures string, step func(string, func(), func() bool) error) error {
	m := d.model
	if err := step("20-crop-drag", func() {
		o := imaging.Defaults()
		o.Format = imaging.PNG
		m.SelectAll()
		m.SetImage(o)
		m.SetOutput(files.Settings{Directory: filepath.Join(fixtures, "compose-export"), Conflict: files.Rename})
		d.importPaths([]string{filepath.Join(fixtures, "compose")})
	}, func() bool {
		if d.page != pictures || m.Scanning || len(m.Sources) != 4 || !m.OriginalReady() {
			return false
		}
		d.resizeTab = 0
		d.navigate(resize)
		d.crop.ratio = crop.FourThree
		d.crop.box = crop.Fit(image.Rectangle{Max: d.crop.size}, d.crop.ratio)
		d.crop.gesture = cropGesture{held: true, handle: 3, start: d.crop.box}
		d.redraw()
		return true
	}); err != nil {
		return err
	}
	if err := step("21-crop-applied", func() { d.endCrop(true) }, func() bool {
		got := m.AfterFrame(true).Info
		return !m.Rendering && got.Width == 872 && got.Height == 654
	}); err != nil {
		return err
	}
	if err := step("22-collage-grid", func() {
		m.ApplyCrop(map[string]bool{m.Selected: true}, crop.Region{})
		d.list = true
		for _, item := range m.Sources {
			d.model.EditSelection[item.Path] = true
		}
		d.beginCollage()
	}, func() bool { return d.page == collage && !m.Collage.Loading && len(m.Collage.Tiles) == 4 }); err != nil {
		return err
	}
	if err := step("23-collage-reordered", func() {
		items := m.Collage.Items
		m.MoveCollage(items[0].Path, items[3].Path)
	}, nil); err != nil {
		return err
	}
	if err := step("24-collage-export", d.exportCollage, func() bool {
		return !m.Collage.Exporting && (m.Collage.LastOutput != "" || m.Error != nil)
	}); err != nil {
		return err
	}
	var path string
	var expected image.Point
	var exportErr error
	mygo.RunOnMain(func() {
		path, exportErr = m.Collage.LastOutput, m.Error
		expected = image.Pt(m.Collage.Options.Width, m.Collage.Options.Height)
	})
	if exportErr != nil {
		return exportErr
	}
	_, info, err := imaging.Thumbnail(context.Background(), path, 64)
	if err != nil || info.Format != imaging.PNG || image.Pt(info.Width, info.Height) != expected {
		return fmt.Errorf("collage output: %+v %v", info, err)
	}
	log.Printf("visual QA: confirmed 4:3 crop and reordered collage export at %v", expected)
	return nil
}

func captureCertifications(d *desktop, fixtures string, step func(string, func(), func() bool) error) error {
	m := d.model
	var base image.Image
	if err := step("25-certification-empty", func() {
		m.SetImage(imaging.Defaults())
		m.SetOutput(files.Settings{Directory: filepath.Join(fixtures, "certification-export"), Conflict: files.Rename})
		d.importPaths([]string{filepath.Join(fixtures, "certification")})
	}, func() bool {
		if m.Scanning || len(m.Sources) != 6 || !m.OriginalReady() {
			return false
		}
		if d.page != certificationPage {
			d.navigate(certificationPage)
		}
		base = m.AfterFrame(true).Image
		return base != nil && !m.Rendering && len(m.CertificationBadges()) == certification.MaxBadges
	}); err != nil {
		return err
	}
	if err := step("26-certification", func() {
		m.SelectAll()
		g, badges := m.Certification(), certification.Builtins()
		g.Toggle(badges[0])
		g.Toggle(badges[3])
		m.SetCertification(g)
	}, nil); err != nil {
		return err
	}
	var stable bool
	mygo.RunOnMain(func() {
		frame := m.AfterFrame(true)
		stable = frame.Image == base && len(frame.Layers) == 2 && !m.Rendering
	})
	if !stable {
		return fmt.Errorf("certification edits replaced the canvas or lost a layer")
	}
	if err := step("27-certification-results", func() { d.start(nil) }, func() bool {
		return !m.Working && m.Progress.Success == 6
	}); err != nil {
		return err
	}
	var marked int
	mygo.RunOnMain(func() {
		for _, result := range m.Report.Results {
			if result.Entry.Source.Certification.Count() == 2 {
				marked++
			}
		}
	})
	if marked != 6 {
		return fmt.Errorf("certification batch: only %d of 6 exports had both badges", marked)
	}
	log.Print("visual QA: stable two-badge preview and six marked exports after Select All")
	return nil
}

func captureCaption(d *desktop, fixtures string, step func(string, func(), func() bool) error) error {
	m := d.model
	const title = "Inline Sump Pump: A Selection Guide\nfor Industrial Buyers"
	paths := []string{filepath.Join(fixtures, "caption-photo.webp")}
	if _, err := os.Stat(paths[0]); errors.Is(err, os.ErrNotExist) {
		paths[0] = filepath.Join(fixtures, "certification", "01-pump.webp")
	} else if err != nil {
		return err
	}
	for _, file := range []string{"02-inlet.jpg", "03-motor.jpg", "04-outlet.jpg", "05-battery.jpg", "06-machine.jpg"} {
		paths = append(paths, filepath.Join(fixtures, "certification", file))
	}
	var base image.Image
	if err := step("28-caption-empty", func() { d.importPaths(paths) }, func() bool {
		if m.Scanning || !m.OriginalReady() || m.Thumbnail(m.Selected).Image == nil {
			return false
		}
		if d.page != captionPage {
			d.navigate(captionPage)
		}
		f := m.AfterFrame(true)
		base = f.Image
		return base != nil && !m.Rendering && len(f.Layers) == 0
	}); err != nil {
		return err
	}
	if err := step("29-caption", func() { m.SetCaption(caption.Caption{Text: title, Style: caption.Defaults()}) }, func() bool {
		f := m.AfterFrame(true)
		return f.Image == base && !m.Rendering && len(f.Layers) == 2 && f.Layers[1].Pixels != nil
	}); err != nil {
		return err
	}
	log.Print("visual QA: title strip overlay added without replacing the preview bitmap")
	if err := step("30-undo-applied", func() { m.SelectAll(); v := m.Caption(); v.Text = title + " ·"; m.SetCaption(v) }, func() bool {
		return m.UndoAction() == app.HistoryCaption && m.AfterFrame(true).Image == base && m.Sources[1].Caption.Text == title+" ·"
	}); err != nil {
		return err
	}
	if err := step("31-redo-ready", func() { d.replayHistory(false) }, func() bool {
		return m.RedoAction() == app.HistoryCaption && m.AfterFrame(true).Image == base && m.Sources[1].Caption.Text == ""
	}); err != nil {
		return err
	}
	return nil
}

func captureSequence(d *desktop, fixtures, target string) error {
	onUI := mygo.RunOnMain
	m := d.model
	var expectedFrame, workspaceFrame mygo.Rectangle
	onUI(func() {
		area := mygo.Screen.PrimaryDisplay().WorkArea
		expectedFrame = modeFrame(true, m.Preferences.WindowSize, area)
		workspaceFrame = initialWindowFrame(m.Preferences.WindowSize, area)
	})
	expectedSize := frameSize(workspaceFrame)
	wait := func(cond func() bool) error {
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			ok := false
			onUI(func() { ok = cond() })
			if ok {
				return nil
			}
			time.Sleep(30 * time.Millisecond)
		}
		var state string
		onUI(func() {
			thumb := m.Thumbnail(m.Selected)
			state = fmt.Sprintf("page=%v sources=%d selected=%q scanning=%t planning=%t rendering=%t scanErrors=%d preview=%t thumb=%t thumbError=%v previewError=%v planError=%v",
				d.page, len(m.Sources), filepath.Base(m.Selected), m.Scanning, m.Planning, m.Rendering, m.Scan.Errors,
				m.Preview.Before != nil, thumb.Image != nil, thumb.Err, m.PreviewErr, m.PlanErr)
		})
		return fmt.Errorf("timed out waiting for the native view (%s)", state)
	}
	capture := func(name string) error {
		// Give the native layout and transitions a frame or two to settle.
		time.Sleep(230 * time.Millisecond)
		if err := wait(func() bool { return !d.geometry.transitioning }); err != nil {
			return err
		}
		want := expectedSize
		onUI(func() {
			if d.page == importHome {
				want = frameSize(expectedFrame)
			}
		})
		if size := frameSize(d.win.Bounds()); size != want {
			return fmt.Errorf("%s: native window size %+v; want %+v", name, size, want)
		}
		png, err := d.win.CapturePage()
		if err != nil {
			return err
		}
		path := filepath.Join(target, name+".png")
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, err = f.Write(png)
		err = errors.Join(err, f.Close())
		if err == nil {
			log.Printf("visual QA: %s", name)
		}
		return err
	}
	step := func(name string, change func(), ready func() bool) error {
		if change != nil {
			onUI(func() { change(); d.redraw() })
		}
		if ready != nil {
			if err := wait(ready); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		return capture(name)
	}
	if err := wait(d.win.IsVisible); err != nil {
		return err
	}
	// AppKit may round a backing-pixel origin to the adjacent point.
	if frame := d.win.Bounds(); frameSize(frame) != frameSize(expectedFrame) ||
		frame.X < expectedFrame.X-1 || frame.X > expectedFrame.X+1 ||
		frame.Y < expectedFrame.Y-1 || frame.Y > expectedFrame.Y+1 {
		return fmt.Errorf("startup window frame %+v; want %+v", frame, expectedFrame)
	}
	log.Printf("visual QA: centered launch at %d × %d pt", expectedFrame.Width, expectedFrame.Height)
	onUI(func() { mygo.Theme.SetSource(mygo.ThemeLight) })
	if os.Getenv("IMAGEFLOW_VISUAL_SCENARIO") == "custom-collage" {
		return captureCustomCollage(d, fixtures, step)
	}
	if os.Getenv("IMAGEFLOW_VISUAL_SCENARIO") == "edit-scope" {
		return captureEditScope(d, fixtures, step)
	}
	if err := capture("01-welcome"); err != nil {
		return err
	}
	if err := step("01-welcome-dark", func() { mygo.Theme.SetSource(mygo.ThemeDark) }, nil); err != nil {
		return err
	}
	if err := step("01a-no-images", func() {
		mygo.Theme.SetSource(mygo.ThemeLight)
		d.importPaths([]string{filepath.Join(fixtures, "empty")})
	}, func() bool { return !m.Scanning && d.page == importHome && d.importAttempted }); err != nil {
		return err
	}

	ready := func() bool { return !m.Scanning && !m.Planning && !m.Rendering && m.PreviewErr == nil }
	mixed := filepath.Join(fixtures, "formats")
	formatOutput := filepath.Join(fixtures, "format-export", "chuli")
	if err := step("01b-mixed-import", func() {
		d.importPaths([]string{mixed})
	}, func() bool {
		return ready() && d.page == pictures && len(m.Sources) == 6 && m.Scan.Errors == 2 &&
			m.Preview.Before != nil && m.Preview.Input.Format == imaging.HEIF
	}); err != nil {
		return err
	}
	if err := step("01c-avif-preview", func() {
		o := imaging.Defaults()
		o.Format = imaging.AVIF
		o.Resize = imaging.Resize{Enabled: true, Mode: imaging.LongEdge, Limit: 720}
		m.SelectAll()
		m.SetImage(o)
		m.SetOutput(files.Settings{Directory: formatOutput, Conflict: files.Rename})
		d.navigate(format)
	}, func() bool {
		return ready() && m.PlanErr == nil && m.Preview.Before != nil && m.Preview.After != nil &&
			m.Preview.Output.Format == imaging.AVIF && m.Preview.Output.Bytes > 0
	}); err != nil {
		return err
	}
	if err := step("01d-avif-results", func() { d.start(nil) }, func() bool {
		return !m.Working && m.Progress.Total == 6 && m.Progress.Success == 6
	}); err != nil {
		return err
	}
	var exports []string
	onUI(func() {
		for _, entry := range m.Report.Results {
			exports = append(exports, entry.Output)
		}
	})
	for _, path := range exports {
		if filepath.Ext(path) != ".avif" {
			return fmt.Errorf("incorrect export name: %s", path)
		}
		if err := imaging.InspectInput(context.Background(), path, imaging.AVIF); err != nil {
			return err
		}
	}
	log.Print("visual QA: six mixed formats imported and exported as AVIF, two incompatible files reported")
	onUI(d.newBatch)

	if err := step("01d1-output-folder-skipped", func() {
		m.SetOutput(files.Settings{Directory: formatOutput, Conflict: files.Rename})
		d.importPaths([]string{filepath.Dir(formatOutput)})
	}, func() bool {
		return !m.Scanning && d.page == importHome && m.Scan.SkippedOutputFolder && len(m.Sources) == 0
	}); err != nil {
		return err
	}
	if err := step("01d2-reimport-output-folder", func() {
		d.importPaths([]string{formatOutput})
	}, func() bool {
		return ready() && d.page == pictures && len(m.Sources) == 6 && m.OriginalReady() &&
			m.Scan.Errors == 0 && l10n.ErrorText(l10n.Default, m.PlanErr) == l10n.T(l10n.Default, "error.outputContainsSource")
	}); err != nil {
		return err
	}
	log.Print("visual QA: parent scan skipped its output, direct folder selection reimported six AVIF files and refused exporting over the sources")
	onUI(d.newBatch)

	extended := filepath.Join(fixtures, "extended")
	svg := filepath.Join(extended, "01-mark.svg")
	raw, err := filepath.Glob(filepath.Join(extended, "06-camera.*"))
	if err != nil {
		return err
	}
	want := 5 + len(raw)
	if err := step("01e-extended-import", func() { d.importPaths([]string{svg, extended}) }, func() bool {
		return ready() && d.page == pictures && len(m.Sources) == want && m.Scan.Errors == 2 &&
			m.Preview.Input.Format == imaging.SVG && m.Preview.Before != nil
	}); err != nil {
		return err
	}
	if err := step("01f-svg-preview", func() {
		o := imaging.Defaults()
		o.Format = imaging.PNG
		o.Resize = imaging.Resize{Enabled: true, Mode: imaging.Exact, Fit: imaging.Contain,
			Width: 720, Height: 480, Transparent: true}
		m.SelectAll()
		m.SetImage(o)
		m.SetOutput(files.Settings{Directory: filepath.Join(fixtures, "extended-export"), Conflict: files.Rename})
		d.navigate(format)
	}, func() bool {
		return ready() && m.PlanErr == nil && m.Preview.Before != nil && m.Preview.After != nil && m.Thumbnail(svg).Image != nil &&
			m.Preview.Output.Width == 720 && m.Preview.Output.Height == 480
	}); err != nil {
		return err
	}
	if len(raw) > 0 {
		if err := step("01g-raw-preview", func() { m.Select(raw[0]) }, func() bool {
			return ready() && m.Preview.Input.Format == imaging.RAW && m.Preview.Before != nil && m.Preview.After != nil
		}); err != nil {
			return err
		}
	}
	if err := step("01h-extended-results", func() { d.start(nil) }, func() bool {
		return !m.Working && m.Progress.Total == want && m.Progress.Success == want
	}); err != nil {
		return err
	}
	log.Printf("visual QA: %d new-format images imported and exported as PNG, two incompatible files reported", want)
	onUI(d.newBatch)

	if err := step("01i-animation-scope", func() {
		d.importPaths([]string{filepath.Join(fixtures, "series")})
	}, func() bool {
		return ready() && len(m.Sources) == 2 && m.Scan.Errors == 0 && m.Preview.Before != nil &&
			m.SelectedSource().Series == (series.Info{Kind: series.Frames, Count: 3}) && m.SelectedSource().Selection.Scope == ""
	}); err != nil {
		return err
	}
	if err := step("01j-scope-required", func() { d.start(nil) }, func() bool {
		return !m.Working && d.seriesPrompt == m.Selected && m.Preview.Before != nil
	}); err != nil {
		return err
	}
	if err := step("01k-animation-range", func() {
		m.SetSeriesSelection(series.Selection{Scope: series.Custom, Range: "1,3"})
		m.SetPreviewPart(3)
	}, func() bool { return ready() && m.PreviewPart == 3 }); err != nil {
		return err
	}
	if err := step("01l-tiff-pages", func() {
		m.Select(m.Sources[1].Path)
		m.SetSeriesSelection(series.Selection{Scope: series.All})
		m.SetPreviewPart(2)
		m.SetOutput(files.Settings{Directory: filepath.Join(fixtures, "series-export"), Conflict: files.Rename})
	}, func() bool {
		return ready() && m.PlanErr == nil && m.Preview.Input.Width == 30 && m.Preview.Input.Height == 54
	}); err != nil {
		return err
	}
	if err := step("01m-series-results", func() { d.start(nil) }, func() bool {
		return !m.Working && m.Progress.Total == 2 && m.Progress.Success == 2
	}); err != nil {
		return err
	}
	for _, want := range []struct {
		name string
		info series.Info
	}{
		{"01-animation.webp", series.Info{Kind: series.Frames, Count: 2}},
		{"02-book.tiff", series.Info{Kind: series.Pages, Count: 3}},
	} {
		path := filepath.Join(fixtures, "series-export", want.name)
		if got, err := imaging.InspectSource(context.Background(), path, imaging.FormatOf(path)); err != nil || got != want.info {
			return fmt.Errorf("%s: got %+v, wanted %+v, error %v", want.name, got, want.info, err)
		}
	}
	log.Print("visual QA: selected GIF frames → 2-frame WebP; all TIFF pages → 3-page TIFF")
	onUI(d.newBatch)

	if err := captureComposition(d, fixtures, step); err != nil {
		return err
	}
	onUI(d.newBatch)
	if err := captureCertifications(d, fixtures, step); err != nil {
		return err
	}
	onUI(d.newBatch)
	if err := captureCaption(d, fixtures, step); err != nil {
		return err
	}
	onUI(d.newBatch)

	source := filepath.Join(fixtures, "images", "01-pump.jpg")
	logo := filepath.Join(fixtures, "logo.png")
	out := files.Settings{Directory: filepath.Join(fixtures, "export"), KeepTree: true, Conflict: files.Rename}
	o := imaging.Defaults()
	r := files.DefaultRules()
	r.KeepOriginal, r.Prefix = false, "QA_"
	dual := o
	dual.Resize.Enabled = true
	dual.Text.Enabled, dual.Text.Text = true, "MOCHIPIC QA"
	dual.Text.Font = "Arial Bold"
	dual.Logo.Enabled, dual.Logo.Path = true, logo
	preset := config.Preset{ID: "visual-qa", Name: "产品图验收", Image: dual, Naming: r}
	if err := step("02-workspace", func() {
		p := m.Preferences.Clone()
		p.Presets = append(p.Presets, preset)
		p.DefaultPreset, p.Output, p.Workers = "builtin-web", out, 4
		m.SetPreferences(p)
		m.ApplyPreset(preset)
		m.SetOutput(out)
		d.importPaths([]string{source, filepath.Join(fixtures, "images")})
	}, func() bool {
		return ready() && d.page == pictures && m.Selected == source && m.OriginalReady() &&
			len(m.Sources) == 13 && m.Thumbnail(m.Sources[2].Path).Image != nil
	}); err != nil {
		return err
	}
	if err := step("02-filmstrip-selection", func() {
		d.pickThumbnail(m.Sources[2].Path, ui.Cmd)
		d.pickThumbnail(m.Sources[4].Path, ui.Cmd)
	}, func() bool {
		return len(d.model.EditSelection) == 3 && m.Selected == source && m.OriginalReady() &&
			m.Thumbnail(m.Sources[4].Path).Image != nil
	}); err != nil {
		return err
	}
	var immediate, reused bool
	if err := step("02a-image-switched", func() {
		d.pickThumbnail(source, 0)
		next := m.Sources[2].Path // Skip the one adjacent photo that may be warmed.
		m.Select(next)
		f := m.BeforeFrame()
		immediate = f.Thumbnail && f.Image == m.Thumbnail(next).Image
	}, func() bool { return ready() && m.OriginalReady() }); err != nil {
		return err
	}
	onUI(func() {
		next, pixels := m.Selected, m.Preview.Before
		m.Select(source)
		m.Select(next)
		reused = m.OriginalReady() && !m.Rendering && m.Preview.Before == pixels
		m.Select(source)
		d.redraw()
	})
	if !immediate || !reused {
		return fmt.Errorf("image switching: immediate selected thumbnail=%t cached clear preview=%t", immediate, reused)
	}
	log.Print("visual QA: cold selection used its own thumbnail, returning used the clear preview immediately")
	if err := step("03-image-library", func() {
		d.list = true
		for _, src := range m.Sources[:min(3, len(m.Sources))] {
			d.model.EditSelection[src.Path] = true
		}
	}, func() bool { return m.Thumbnail(source).Image != nil }); err != nil {
		return err
	}
	textOnly := dual
	textOnly.Logo.Enabled, textOnly.Logo.Path = false, ""
	if err := step("04a-empty-text-keeps-logo", func() {
		draft := dual
		draft.Text.Text = ""
		d.list = false
		d.watermarkTab = 0
		m.SetImage(draft)
		d.navigate(watermark)
	}, func() bool {
		f := m.AfterFrame(true)
		return ready() && f.Image != nil && len(f.Layers) == 1 && !m.Image.Text.Enabled
	}); err != nil {
		return err
	}
	if err := step("04-text-watermark", func() { d.list = false; m.SetImage(textOnly); d.navigate(watermark); d.watermarkTab = 0 }, func() bool { return ready() && len(m.AfterFrame(true).Layers) == 1 }); err != nil {
		return err
	}
	if err := step("05a-logo-chooser", func() {
		m.SetImage(textOnly)
		d.watermarkTab = 1
	}, func() bool {
		f := m.AfterFrame(true)
		return ready() && f.Image != nil && len(f.Layers) == 1 && !m.Image.Logo.Enabled
	}); err != nil {
		return err
	}
	if err := step("05-logo-watermark", func() {
		logoOnly := dual
		logoOnly.Text.Text, logoOnly.Logo.Path = "", ""
		m.SetImage(logoOnly)
		d.watermarkTab = 1
		m.AddLogos([]string{logo})
	}, func() bool {
		return ready() && !m.LogoPending && m.Image.Logo.Enabled && len(m.AfterFrame(true).Layers) == 1 && m.LogoThumbnail(logo) != nil
	}); err != nil {
		return err
	}
	brand := filepath.Join(fixtures, "logos", "brand-logo.png")
	dealer := filepath.Join(fixtures, "logos", "dealer-logo.png")
	if err := step("05b-multi-logo", func() {
		d.watermarkTab = 1
		logoOnly := o
		logoOnly.Logo.Width, logoOnly.Logo.Opacity = 15, 100
		m.SetImage(logoOnly)
		m.AddLogos([]string{brand, dealer})
	}, func() bool {
		return ready() && !m.LogoPending && len(m.AfterFrame(true).Layers) == 2 && d.logoSelection == dealer && m.LogoThumbnail(brand) != nil
	}); err != nil {
		return err
	}
	if err := step("05c-logo-deselected", func() { d.logoSelection = "" }, func() bool {
		return ready() && len(m.AfterFrame(true).Layers) == 2 && d.logoSelection == ""
	}); err != nil {
		return err
	}
	if err := step("06-combined-watermarks", func() { m.SetImage(dual); d.watermarkTab = 2 }, func() bool {
		return ready() && len(m.AfterFrame(true).Layers) == 2
	}); err != nil {
		return err
	}
	if err := step("06a-free-watermark-positions", func() {
		opts, frame := m.Image, m.AfterFrame(true)
		size := image.Pt(frame.Info.Width, frame.Info.Height)
		for _, layer := range frame.Layers {
			switch layer.Mark {
			case imaging.TextWatermark:
				opts.Text.MoveTo(image.Pt(size.X/3, size.Y*3/4), size, layer.Bounds.Size())
			case imaging.LogoWatermark:
				opts.Logo.MoveTo(image.Pt(size.X/5, size.Y/5), size, layer.Bounds.Size())
			}
		}
		m.SetImage(opts)
		d.watermarkTab = 0
	}, func() bool {
		return ready() && m.Image.Text.Free.Active && m.Image.Logo.Free.Active
	}); err != nil {
		return err
	}
	resized := o
	resized.Resize.Enabled = true
	if err := step("07-resize", func() { m.SetImage(resized); d.resizeTab = 1; d.navigate(resize) }, func() bool {
		out := m.AfterFrame(true).Info
		return ready() && out.Width == 1200 && out.Height == 1200
	}); err != nil {
		return err
	}
	if err := step("08-format", func() { m.SetImage(o); d.navigate(format) }, func() bool {
		return ready() && m.Preview.Output.Format == imaging.WebP && m.Preview.Output.Bytes > 0
	}); err != nil {
		return err
	}

	if err := step("09-rename", func() { m.SetNaming(r); d.navigate(rename) }, func() bool {
		return !m.Planning && len(m.Plan.Entries) > 0 && m.Thumbnail(source).Image != nil
	}); err != nil {
		return err
	}
	if err := step("10-encoded-result", func() {
		m.SetImage(dual)
		d.navigate(format)
	}, func() bool { return ready() && m.Preview.Before != nil && m.Preview.After != nil }); err != nil {
		return err
	}
	if err := step("11-export", func() { m.SetOutput(out); d.navigate(output) }, func() bool { return !m.Planning && m.PlanErr == nil }); err != nil {
		return err
	}
	onUI(func() { m.Import([]string{filepath.Join(fixtures, "batch")}); d.redraw() })
	if err := wait(func() bool { return !m.Scanning && len(m.Sources) > 100 }); err != nil {
		return err
	}
	onUI(func() {
		m.SetImage(dual)
		d.start(nil)
	})
	if err := wait(func() bool { return m.Working && m.Progress.Success > 0 && m.Progress.Running > 0 }); err != nil {
		return err
	}
	if err := capture("12-processing"); err != nil {
		return err
	}
	if err := wait(func() bool {
		return !m.Working && m.Progress.Success == 178 && m.Progress.Failure == 5 && m.Progress.Total == 183
	}); err != nil {
		return err
	}
	log.Print("visual QA: completed 183 items (178 outputs, 5 damaged inputs)")
	if err := capture("13-completed"); err != nil {
		return err
	}
	if err := step("14-errors", func() {
		d.navigate(failures)
		d.errorFocus = m.Failures[0].Entry.Source.Path
		d.errorSelected[d.errorFocus] = true
	}, nil); err != nil {
		return err
	}
	if err := step("15-presets", func() { d.navigate(presets); d.chosenPreset = "builtin-social" }, nil); err != nil {
		return err
	}
	if err := step("16-settings", func() { d.navigate(settings) }, nil); err != nil {
		return err
	}
	if err := step("16-settings-dark", func() {
		p := m.Preferences
		p.Theme = "dark"
		m.SetPreferences(p)
		mygo.Theme.SetSource(mygo.ThemeDark)
	}, nil); err != nil {
		return err
	}
	onUI(func() {
		mygo.Theme.SetSource(mygo.ThemeLight)
		d.navigate(failures)
		d.retrySelected(false)
	})
	if err := wait(func() bool {
		return !m.Working && m.Progress.Total == 1 && m.Progress.Failure == 1 && len(m.Failures) == 5
	}); err != nil {
		return fmt.Errorf("retry selected: %w", err)
	}
	log.Print("visual QA: selected retry ran one item and retained the other failures")
	onUI(func() {
		out.Directory = filepath.Join(fixtures, "cancel")
		m.SetOutput(out)
		d.start(nil)
	})
	if err := wait(func() bool { return m.Working && m.Progress.Success > 0 }); err != nil {
		return fmt.Errorf("start cancellation run: %w", err)
	}
	onUI(m.Cancel)
	if err := wait(func() bool {
		return !m.Working && m.Progress.Success > 0 && m.Progress.Cancelled > 0 && m.Progress.Done() == m.Progress.Total
	}); err != nil {
		return fmt.Errorf("finish cancellation run: %w", err)
	}
	onUI(func() {
		p := m.Progress
		log.Printf("visual QA: cancelled run: %d success, %d failure, %d skipped, %d cancelled", p.Success, p.Failure, p.Skipped, p.Cancelled)
	})
	if err := capture("17-cancelled"); err != nil {
		return err
	}
	onUI(func() {
		expectedSize = windowMinimum(mygo.Screen.DisplayMatching(d.win.Bounds()).WorkArea)
		d.win.SetSize(expectedSize.Width, expectedSize.Height)
	})
	for _, lang := range l10n.Languages() {
		if lang.Tag == l10n.Default {
			continue
		}
		if err := step("18-settings-"+lang.Tag, func() {
			p := m.Preferences
			p.Language, p.Theme = lang.Tag, "light"
			d.applySettings(p)
			d.navigate(settings)
		}, func() bool {
			items := mygo.App.Menu().Items()
			return mygo.App.Locale() == lang.Tag && len(items) > 1 && items[1].Label == l10n.T(lang.Tag, "menu.file")
		}); err != nil {
			return err
		}
		if err := step("19-watermark-"+lang.Tag, func() {
			d.navigate(watermark)
			d.watermarkTab = 0
		}, func() bool { return ready() && m.AfterFrame(true).Image != nil }); err != nil {
			return err
		}
	}
	return nil
}

func captureCustomCollage(d *desktop, fixtures string, step func(string, func(), func() bool) error) error {
	m := d.model
	s := &m.Collage
	if err := step("01-selected-photos", func() { d.importPaths([]string{filepath.Join(fixtures, "images")}) }, func() bool {
		if m.Scanning || len(m.Sources) != 3 || !m.OriginalReady() {
			return false
		}
		m.SelectAll()
		return true
	}); err != nil {
		return err
	}
	if err := step("02-grid", d.beginCollage, func() bool { return !s.Loading && len(s.Tiles) == 3 }); err != nil {
		return err
	}
	if err := step("03-custom", func() {
		o := s.Options
		o.Layout = imaging.CollageCustom
		m.SetCollageOptions(o)
		o = s.Options
		size := image.Pt(o.Width, o.Height)
		gap := o.Gap
		split := size.X * 56 / 100
		mid := size.Y / 2
		boxes := []image.Rectangle{image.Rect(gap, gap, split-gap/2, size.Y-gap), image.Rect(split+gap/2, gap, size.X-gap, mid-gap/2), image.Rect(split+gap/2, mid+gap/2, size.X-gap, size.Y-gap)}
		for i, item := range s.Items {
			tile := s.Tiles[item.Path]
			fit := o
			fit.Fit = imaging.Cover
			p := imaging.NewCollagePhoto(item.Path, size, tile.Size, fit.Place(boxes[i], tile.Size))
			o.PutPhoto(p)
		}
		m.SetCollageOptions(o)
		d.collage.selected = s.Items[0].Path
	}, nil); err != nil {
		return err
	}
	if err := step("04-corners", func() {
		o := s.Options
		p, _ := o.Photo(s.Items[0].Path)
		p = p.Adjust(1, image.Pt(-32, 40), false)
		p.Corner = imaging.CornerRound
		p.Radius = 52
		o.PutPhoto(p)
		m.SetCollageOptions(o)
	}, nil); err != nil {
		return err
	}
	return step("05-export", func() {
		o := m.Image
		o.Format = imaging.PNG
		m.SetImage(o)
		m.SetOutput(files.Settings{Directory: filepath.Join(fixtures, "export"), Conflict: files.Rename})
		d.exportCollage()
	}, func() bool { return !s.Exporting && s.LastOutput != "" })
}
