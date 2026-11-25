package batch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"imageflow/internal/caption"
	"imageflow/internal/certification"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/series"
	"imageflow/internal/testkit"
)

func TestMain(m *testing.M) {
	if err := imaging.Start(); err != nil {
		panic(err)
	}
	code := m.Run()
	imaging.Shutdown()
	os.Exit(code)
}

func mkdir(t *testing.T, p string) string {
	t.Helper()
	if e := os.MkdirAll(p, 0o700); e != nil {
		t.Fatal(e)
	}
	return p
}
func write(t *testing.T, p string, b []byte) {
	t.Helper()
	testkit.Write(t, p, func(w io.Writer) error { _, err := w.Write(b); return err })
}
func plan(t *testing.T, src []files.Source, dir string, policy files.Conflict, tree bool) files.Plan {
	t.Helper()
	p, e := files.BuildPlan(t.Context(), src, files.Settings{Directory: dir, Conflict: policy, KeepTree: tree}, files.DefaultRules(), ".webp", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func assertProgress(t *testing.T, p Progress, maxWorkers int) {
	t.Helper()
	if p.Done()+p.Running+p.Remaining != p.Total || p.Running > maxWorkers || p.Remaining < 0 || p.Running < 0 {
		t.Errorf("invalid progress: %+v", p)
	}
}
func assertUntouched(t *testing.T, sums map[string][32]byte) {
	t.Helper()
	for p, want := range sums {
		if testkit.Hash(t, p) != want {
			t.Fatalf("source changed: %s", p)
		}
	}
}
func assertClean(t *testing.T, dir string, count int) {
	t.Helper()
	n := 0
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if filepath.Ext(path) == ".tmp" {
				t.Errorf("partial file: %s", path)
			}
			if filepath.Ext(path) == ".webp" {
				n++
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n != count {
		t.Fatalf("expected %d WebP files, found %d", count, n)
	}
}

func TestScanMixedAndAvoidOutput(t *testing.T) {
	base := t.TempDir()
	in := mkdir(t, filepath.Join(base, "input"))
	sub := mkdir(t, filepath.Join(in, "产品"))
	out := mkdir(t, filepath.Join(in, "导出"))
	a, b := filepath.Join(in, "a.png"), filepath.Join(sub, "b.JPEG")
	img := testkit.Solid(21, 19, color.NRGBA{R: 180, A: 255})
	testkit.PNG(t, a, img)
	testkit.JPEG(t, b, img)
	write(t, filepath.Join(in, "note.txt"), []byte("unsupported"))
	write(t, filepath.Join(out, "result.webp"), []byte("not an input"))
	if e := os.Link(a, filepath.Join(sub, "alias.png")); e != nil {
		t.Fatal(e)
	}
	testkit.DirectoryLink(t, sub, filepath.Join(in, "link"))
	out, _ = filepath.EvalSymlinks(out)
	r := Scan(t.Context(), []string{a, in, sub}, nil, ScanOptions{Recursive: true, Exclude: []string{out}})
	if len(r.Added) != 2 || r.Duplicates < 2 || r.Added[1].RelativeDir != "产品" || r.Errors != 0 {
		t.Fatalf("%+v", r)
	}
	if got := Scan(t.Context(), []string{in}, nil, ScanOptions{Exclude: []string{out}}); len(got.Added) != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestScanAllowsExplicitOutputSelectionButSkipsNestedOutputs(t *testing.T) {
	root := t.TempDir()
	out := mkdir(t, filepath.Join(root, "chuli"))
	nested := mkdir(t, filepath.Join(out, "next"))
	first, second := filepath.Join(out, "first.jpg"), filepath.Join(nested, "second.jpg")
	im := testkit.Solid(32, 26, color.NRGBA{R: 180, A: 255})
	testkit.JPEG(t, first, im)
	testkit.JPEG(t, second, im)
	canonical := func(p string) string {
		t.Helper()
		got, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	for _, tc := range []struct {
		name         string
		path         string
		want         string
		skipped      bool
		nonRecursive bool
	}{
		{name: "scan parent", path: root, skipped: true},
		{name: "choose output folder", path: out, want: first, skipped: true},
		{name: "choose output folder without recursion", path: out, want: first, skipped: true, nonRecursive: true},
		{name: "choose nested output folder", path: nested, want: second},
		{name: "choose one exported file", path: second, want: second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := Scan(t.Context(), []string{tc.path}, nil, ScanOptions{
				Recursive: !tc.nonRecursive, Exclude: []string{canonical(out), canonical(nested)},
			})
			if report.Errors != 0 || report.SkippedOutputFolder != tc.skipped {
				t.Fatalf("unexpected report: %+v", report)
			}
			if tc.want == "" {
				if len(report.Added) != 0 {
					t.Fatalf("parent scan imported its own outputs: %+v", report.Added)
				}
			} else if len(report.Added) != 1 || report.Added[0].Path != canonical(tc.want) {
				t.Fatalf("did not import the explicitly selected image: %+v", report.Added)
			}
		})
	}
}

func TestMixedCodecsAVIF(t *testing.T) {
	root := t.TempDir()
	in := mkdir(t, filepath.Join(root, "输入"))
	out := mkdir(t, filepath.Join(root, "输出"))
	sums := map[string][32]byte{}
	for _, name := range []string{"apple.heic", "raster.bmp", "single.tiff", "still.gif", "two-sizes.ico", "two-pages.tiff"} {
		data, err := os.ReadFile(filepath.Join("..", "imaging", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(in, name)
		write(t, path, data)
		sums[path] = testkit.Hash(t, path)
	}
	corrupt := filepath.Join(in, "坏图.avif")
	write(t, corrupt, []byte("invalid AVIF content"))
	sums[corrupt] = testkit.Hash(t, corrupt)
	report := Scan(t.Context(), []string{in}, nil, ScanOptions{})
	if len(report.Added) != 6 || report.Errors != 1 {
		t.Fatalf("unexpected import report: %+v", report)
	}
	if len(report.Issues) != 1 || filepath.Base(report.Issues[0].Path) != filepath.Base(corrupt) {
		t.Fatalf("import issues: %+v", report.Issues)
	}
	for i, s := range report.Added {
		if s.Series.Multiple() {
			report.Added[i].Selection = series.Selection{Scope: series.All, Separate: true}
		}
	}
	o := imaging.Defaults()
	o.Format = imaging.AVIF
	o.Resize = imaging.Resize{Enabled: true, Mode: imaging.Exact, Fit: imaging.Contain,
		Width: 40, Height: 40, Transparent: true}
	p, err := files.BuildPlan(t.Context(), report.Added, files.Settings{Directory: out, Conflict: files.Rename},
		files.DefaultRules(), o.Format.Extension(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	batch, err := Run(t.Context(), p, o, 4, nil)
	if err != nil || batch.Success != 7 || batch.Done() != 7 {
		t.Fatalf("batch: %+v %v", batch, err)
	}
	for _, result := range batch.Results {
		image, info, err := imaging.Thumbnail(t.Context(), result.Output, 64)
		if filepath.Ext(result.Output) != ".avif" || err != nil || image == nil ||
			info.Format != imaging.AVIF || info.Width != 40 || info.Height != 40 {
			t.Fatalf("invalid output %s: %+v %v", result.Output, info, err)
		}
	}
	items, err := os.ReadDir(out)
	if err != nil || len(items) != 7 {
		t.Fatalf("unexpected output files: %v %v", items, err)
	}
	assertUntouched(t, sums)
}

func TestSeriesPlanBatchAndPartRetry(t *testing.T) {
	out, in := t.TempDir(), t.TempDir()
	g, book := filepath.Join(in, "Motion.gif"), filepath.Join(in, "Book.tiff")
	for dest, fixture := range map[string]string{g: "timed.gif", book: "different-pages.tiff"} {
		b, err := os.ReadFile(filepath.Join("..", "imaging", "testdata", fixture))
		if err != nil {
			t.Fatal(err)
		}
		write(t, dest, b)
	}
	sums := map[string][32]byte{g: testkit.Hash(t, g), book: testkit.Hash(t, book)}
	prior := filepath.Join(out, "MOTION-FRAME-0001.PNG")
	write(t, prior, []byte("existing unrelated output"))
	sums[prior] = testkit.Hash(t, prior)
	items := Scan(t.Context(), []string{g, book}, nil, ScanOptions{})
	if items.Errors != 0 || len(items.Added) != 2 || items.Added[0].Series.Kind != series.Frames || items.Added[1].Series.Kind != series.Pages {
		t.Fatalf("import: %+v", items)
	}
	opts := imaging.Defaults()
	opts.Format = imaging.PNG
	now := time.Now()
	planFor := func(src []files.Source) (files.Plan, error) {
		return files.BuildPlan(t.Context(), src, files.Settings{Directory: out, Conflict: files.Rename}, files.DefaultRules(), ".png", now)
	}
	if _, err := planFor(items.Added); err == nil || !bytes.Contains([]byte(l10n.ErrorText("en", err)), []byte("Motion.gif")) {
		t.Fatalf("must choose the scope before export: %v", err)
	}
	items.Added[0].Selection = series.Selection{Scope: series.Custom, Range: "3, 1-2, 1", Separate: true}
	items.Added[1].Selection = series.Selection{Scope: series.All}
	p, err := planFor(items.Added)
	if err != nil || len(p.Entries) != 4 {
		t.Fatalf("plan: %+v %v", p, err)
	}
	for i, name := range []string{"Motion-frame-0001 (1).png", "Motion-frame-0002.png", "Motion-frame-0003.png", "Book.tiff"} {
		if p.Entries[i].Relative != name {
			t.Fatalf("entry %d: %+v", i, p.Entries[i])
		}
	}
	// Only frame 2 encounters a planned destination problem. The other
	// outputs stay committed and a retry exports exactly that frame.
	p.Entries[1].Issue = errors.New("simulated destination failure")
	r, err := Run(t.Context(), p, opts, 2, nil)
	if err != nil || r.Success != 3 || r.Failure != 1 || r.Done() != 4 || r.Results[1].Stage != StagePlan {
		t.Fatalf("batch: %+v %v", r, err)
	}
	for _, i := range []int{0, 2, 3} {
		sums[r.Results[i].Output] = testkit.Hash(t, r.Results[i].Output)
	}
	if got, err := imaging.InspectSource(t.Context(), r.Results[3].Output, imaging.TIFF); err != nil || got.Count != 3 {
		t.Fatalf("all TIFF pages: %+v %v", got, err)
	}
	retry, err := planFor(r.FailedSources())
	if err != nil || len(retry.Entries) != 1 || retry.Entries[0].Source.Part != 2 {
		t.Fatalf("retry plan: %+v %v", retry, err)
	}
	r, err = Run(t.Context(), retry, opts, 2, nil)
	if err != nil || r.Success != 1 || r.Results[0].Output == "" {
		t.Fatalf("retry result: %+v %v", r, err)
	}
	frame, _, err := imaging.Thumbnail(t.Context(), r.Results[0].Output, 64)
	if err != nil || color.NRGBAModel.Convert(frame.At(7, 7)).(color.NRGBA).G < 180 {
		t.Fatalf("retry did not render frame 2: %v", err)
	}
	assertUntouched(t, sums)
}

func TestMixedResultsAndRetry(t *testing.T) {
	root := t.TempDir()
	in := mkdir(t, filepath.Join(root, "photos"))
	out := mkdir(t, filepath.Join(root, "output"))
	retry := mkdir(t, filepath.Join(root, "retry"))
	img := testkit.Solid(127, 59, color.NRGBA{R: 40, G: 80, B: 120, A: 255})
	sums := map[string][32]byte{}
	var sources []files.Source
	for i, n := range []string{"a.png", "broken.jpg", "c.png", "d.png"} {
		name := filepath.Join(in, n)
		s := files.Source{Path: name, Ordinal: i}
		if i == 1 {
			write(t, name, []byte("corrupt"))
		} else {
			testkit.PNG(t, name, img)
		}
		if i == 3 {
			s.RelativeDir = "blocked"
		}
		sources = append(sources, s)
		sums[name] = testkit.Hash(t, name)
	}
	prior := filepath.Join(out, "C.WEBP")
	write(t, prior, []byte("prior file"))
	priorHash := testkit.Hash(t, prior)
	testkit.DirectoryLink(t, in, filepath.Join(out, "blocked"))
	r, e := Run(t.Context(), plan(t, sources, out, files.Skip, true), imaging.Defaults(), 2, func(p Progress) { assertProgress(t, p, 2) })
	if e != nil {
		t.Fatal(e)
	}
	if r.Success != 1 || r.Failure != 2 || r.Skipped != 1 || r.Cancelled != 0 || r.Done() != 4 {
		t.Fatalf("%+v", r.Progress)
	}
	if len(r.FailedSources()) != 2 || r.Results[1].Stage != string(imaging.Decode) || r.Results[3].Stage != StagePlan {
		t.Fatalf("%+v", r.Results)
	}
	recovered, e := Run(t.Context(), plan(t, r.FailedSources(), retry, files.Rename, true), imaging.Defaults(), 1, nil)
	if e != nil || recovered.Success != 1 || recovered.Failure != 1 {
		t.Fatalf("%+v %v", recovered, e)
	}
	if testkit.Hash(t, prior) != priorHash {
		t.Fatal("overwrote prior result")
	}
	assertUntouched(t, sums)
	assertClean(t, retry, 1)
}

func TestBoundedWorkersAndCancel(t *testing.T) {
	tmp := t.TempDir()
	out := mkdir(t, filepath.Join(tmp, "output"))
	img := testkit.Solid(23, 17, color.NRGBA{R: 255, A: 255})
	sums := map[string][32]byte{}
	var src []files.Source
	for i := range 12 {
		p := filepath.Join(tmp, fmt.Sprintf("source-%02d.png", i))
		testkit.PNG(t, p, img)
		src = append(src, files.Source{Path: p, Ordinal: i})
		sums[p] = testkit.Hash(t, p)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	var active, peak, started atomic.Int32
	ready := make(chan struct{}, 2)
	go func() {
		for range 2 {
			select {
			case <-ready:
			case <-ctx.Done():
				return
			}
		}
		cancel()
	}()
	r, e := run(ctx, plan(t, src, out, files.Rename, false), imaging.Defaults(), 2, func(p Progress) { assertProgress(t, p, 2) }, func(ctx context.Context, _ string, _ imaging.Options, w io.Writer) (imaging.Info, error) {
		started.Add(1)
		n := active.Add(1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		defer active.Add(-1)
		_, _ = w.Write([]byte("partial"))
		ready <- struct{}{}
		<-ctx.Done()
		return imaging.Info{}, ctx.Err()
	})
	if e != nil {
		t.Fatal(e)
	}
	if r.Cancelled != 12 || r.Success != 0 || started.Load() != 2 || peak.Load() != 2 {
		t.Fatalf("%+v started=%d peak=%d", r.Progress, started.Load(), peak.Load())
	}
	assertUntouched(t, sums)
	assertClean(t, out, 0)
}

func TestStress1000(t *testing.T) {
	if os.Getenv("IMAGEFLOW_STRESS") != "1" {
		t.Skip("enable with IMAGEFLOW_STRESS=1")
	}
	root := t.TempDir()
	in := mkdir(t, filepath.Join(root, "sources"))
	out := mkdir(t, filepath.Join(root, "outputs"))
	cancelled := mkdir(t, filepath.Join(root, "cancelled"))
	var fixtures [][]byte
	for seed := range 12 {
		im := testkit.Solid(1200, 800, color.NRGBA{})
		for y := range 800 {
			for x := range 1200 {
				stripe := (x/13 + y/19 + seed) % 21
				im.SetNRGBA(x, y, color.NRGBA{R: uint8((x/5 + stripe*3) % 240), G: uint8((y/4 + stripe*4) % 240), B: uint8((x/9 + y/7 + seed*9) % 245), A: 255})
			}
		}
		var b bytes.Buffer
		if err := jpeg.Encode(&b, im, &jpeg.Options{Quality: 91}); err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, b.Bytes())
	}
	sums := make(map[string][32]byte, 1000)
	for i := range 1000 {
		folder := mkdir(t, filepath.Join(in, fmt.Sprintf("组-%02d", i/100)))
		p := filepath.Join(folder, fmt.Sprintf("样品-%04d.jpg", i))
		write(t, p, fixtures[i%len(fixtures)])
		canonical, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatal(err)
		}
		sums[canonical] = testkit.Hash(t, p)
	}
	scan := Scan(t.Context(), []string{in}, nil, ScanOptions{Recursive: true, Exclude: []string{out}})
	if len(scan.Added) != 1000 {
		t.Fatalf("scan: %d", len(scan.Added))
	}
	o := imaging.Defaults()
	o.Resize.Enabled, o.Resize.Width, o.Resize.Height = true, 720, 720
	o.Text.Enabled, o.Text.Text, o.Text.Font, o.Text.Size = true, "ImageFlow 验证", "PingFang SC", 28
	overlays := "Pango watermark"
	if os.Getenv("IMAGEFLOW_STRESS_CAPTION") == "1" {
		for i := range scan.Added {
			scan.Added[i].Caption = caption.Caption{Text: fmt.Sprintf("Industrial Pump %04d\nSelection guide · 扬程与流量", i+1), Style: caption.Defaults()}
		}
		overlays += " and a unique two-line title strip per image"
	}
	if os.Getenv("IMAGEFLOW_STRESS_LOGOS") == "1" {
		var marks []imaging.LogoMark
		for i, c := range []color.NRGBA{{R: 215, A: 255}, {G: 225, A: 255}} {
			logo := testkit.Solid(1200, 360, c)
			for y := range 360 {
				for x := range 1200 {
					if x%200 > 130 || y%120 > 76 {
						logo.SetNRGBA(x, y, color.NRGBA{})
					}
				}
			}
			path := filepath.Join(root, fmt.Sprintf("logo-%d.png", i))
			testkit.PNG(t, path, logo)
			sums[path] = testkit.Hash(t, path)
			mark := imaging.DefaultLogo()
			mark.Path, mark.Enabled, mark.Anchor = path, true, i*8
			marks = append(marks, mark)
		}
		o.SetLogoMarks(marks)
		overlays += " and two transparent Logos"
	}
	if os.Getenv("IMAGEFLOW_STRESS_BADGES") == "1" {
		g, badges := certification.Defaults(), certification.Builtins()
		g.Toggle(badges[0])
		g.Toggle(badges[3])
		for i := range scan.Added {
			scan.Added[i].Certification = g
		}
		overlays += " and two certification badges"
	}
	// Four independently edited groups share their immutable settings; exports
	// keep the same canvas and use different quality settings per group.
	var settings [4]files.PhotoSettings
	for i := range settings {
		opts := o
		opts.Quality = 79 + 2*i
		settings[i] = files.PhotoSettings{Image: opts, Naming: files.DefaultRules()}
	}
	for i := range scan.Added {
		scan.Added[i].Settings = &settings[i%len(settings)]
	}
	workers := 4
	p := plan(t, scan.Added, out, files.Rename, true)
	start := time.Now()
	r, err := Run(t.Context(), p, o, workers, func(p Progress) { assertProgress(t, p, workers) })
	dur := time.Since(start)
	if err != nil || r.Success != 1000 || r.Failure != 0 {
		for _, item := range r.Results {
			if item.Err != nil {
				t.Log(item.Err)
				break
			}
		}
		t.Fatalf("stress: %+v %v", r.Progress, err)
	}
	assertClean(t, out, 1000)
	assertUntouched(t, sums)
	t.Logf("1000 JPG 1200x800 → WebP 720x720 + %s, %d workers, %s (%.1f img/s), %d failed; %d input SHA-256 unchanged", overlays, workers, dur.Round(time.Millisecond), 1000/dur.Seconds(), r.Failure, len(sums))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p = plan(t, scan.Added, cancelled, files.Rename, true)
	r, err = Run(ctx, p, o, workers, func(p Progress) {
		assertProgress(t, p, workers)
		if p.Success > 0 {
			cancel()
		}
	})
	if err != nil || r.Success < 1 || r.Cancelled < 1 || r.Failure > 0 || r.Done() != 1000 {
		t.Fatalf("cancel: %+v %v", r.Progress, err)
	}
	assertClean(t, cancelled, r.Success)
	assertUntouched(t, sums)
	t.Logf("actual vips cancellation: %d published, %d cancelled, 0 failed; all SHA-256 unchanged", r.Success, r.Cancelled)
	reportStressMemory(t)
}
