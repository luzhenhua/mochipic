package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"imageflow/internal/app"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/series"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func testWorkspace(t *testing.T) (*desktop, *native.Tester, func(string, func() bool)) {
	t.Helper()
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	updates := make(chan func(), 128)
	d := &desktop{errorSelected: map[string]bool{}}
	d.model = app.New(func(fn func()) { updates <- fn }, d.redraw)
	t.Cleanup(func() { _ = d.model.Close() })
	w := native.NewTester(d.view, 1120, 720)
	wait := func(reason string, cond func() bool) {
		t.Helper()
		deadline := time.After(6 * time.Second)
		for !cond() {
			select {
			case fn := <-updates:
				fn()
				w.Frame()
			case <-deadline:
				t.Fatalf("%s: preview=%v plan=%v run=%v", reason, d.model.PreviewErr, d.model.PlanErr, d.model.Error)
			}
		}
	}
	return d, w, wait
}

func clickWorkspace(t *testing.T, w *native.Tester, name string) {
	t.Helper()
	if err := w.Click(name); err != nil {
		t.Fatal(err)
	}
	w.Frame()
}

func TestNativeSeriesScopePreviewAndBatch(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	fixture := func(name string) string {
		p, err := filepath.Abs(filepath.Join("..", "imaging", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	gifPath, tiffPath := fixture("timed.gif"), fixture("different-pages.tiff")
	gifHash, tiffHash := testkit.Hash(t, gifPath), testkit.Hash(t, tiffPath)
	out := t.TempDir()
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename})
	w.DropFiles(560, 365, gifPath, tiffPath)
	w.Frame()
	wait("two series imported", func() bool { return d.page == pictures && len(m.Sources) == 2 && m.Preview.Before != nil })
	if m.SelectedSource().Selection.Scope != "" || !w.HasText("全部帧") || !w.HasText("指定帧") {
		t.Fatalf("frame scope must be visible and unset on import: %+v, text %v", m.SelectedSource(), w.Texts())
	}
	clickWorkspace(t, w, "下一帧")
	wait("second frame displayed", func() bool {
		return m.PreviewPart == 2 && !m.Rendering && m.Preview.Before != nil && color.NRGBAModel.Convert(m.Preview.Before.At(7, 7)).(color.NRGBA).G > 180
	})
	clickWorkspace(t, w, "导出全部 2 张")
	if m.Working || !w.HasText(l10n.T(l10n.Default, "error.seriesScope")) || m.PreviewPart != 2 {
		t.Fatal("start without a scope must keep the current frame and guide the user")
	}
	clickWorkspace(t, w, "全部帧")
	if m.SelectedSource().Selection.Scope != series.All || !w.HasText("合为一个 WebP") {
		t.Fatal("all frames must use a single animated WebP by default")
	}
	clickWorkspace(t, w, "导出全部 2 张") // The next file still needs a decision.
	wait("TIFF needs its own range", func() bool {
		return filepath.Base(m.Selected) == filepath.Base(tiffPath) && !m.Rendering && m.Preview.Before != nil
	})
	if m.Working || !w.HasText("指定页") || !w.HasText(l10n.T(l10n.Default, "error.seriesScope")) {
		t.Fatal("scope from the first file must not be applied to the second")
	}
	clickWorkspace(t, w, "指定页")
	if !w.Focused("编号或范围") {
		t.Fatal("choosing specific pages should focus the range input")
	}
	w.Type("9")
	w.Frame()
	if _, err := m.SelectedSource().Selection.Parts(m.SelectedSource().Series); err == nil {
		t.Fatal("out-of-range page accepted")
	}
	w.Key(native.Cmd, native.KeyA)
	w.Type("2-3")
	w.Frame()
	if m.SelectedSource().Selection.Range != "2-3" {
		t.Fatalf("range editor state: %+v", m.SelectedSource().Selection)
	}
	clickWorkspace(t, w, "合为一个 TIFF")
	clickWorkspace(t, w, "拆成单张 · WebP")
	wait("three planned outputs", func() bool { return !m.Planning && m.PlanErr == nil && len(m.Plan.Entries) == 3 })
	clickWorkspace(t, w, "导出全部 2 张")
	wait("batch finished", func() bool { return !m.Working && m.Progress.Done() == 3 })
	if m.Progress.Success != 3 {
		t.Fatalf("batch result: %+v", m.Report)
	}
	if got, err := imaging.InspectSource(t.Context(), filepath.Join(out, "timed.webp"), imaging.WebP); err != nil || got.Count != 3 {
		t.Fatalf("all three frames must be present: %+v %v", got, err)
	}
	for _, name := range []string{"different-pages-page-0002.webp", "different-pages-page-0003.webp"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatal(err)
		}
	}
	if testkit.Hash(t, gifPath) != gifHash || testkit.Hash(t, tiffPath) != tiffHash {
		t.Fatal("source files changed")
	}
}

func TestNativeImagesAndWatermarkKeepDrawingTheSameBase(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	path := filepath.Join(t.TempDir(), "blue.png")
	testkit.PNG(t, path, testkit.Solid(440, 286, color.NRGBA{R: 22, G: 40, B: 206, A: 255}))
	d.importPaths([]string{path})
	w.Frame()
	wait("original preview", func() bool { return m.Preview.Before != nil && !m.Rendering })
	base := m.Preview.Before
	d.setZoom(1.3)
	w.Frame()
	pixel := func() color.RGBA { return w.Image().RGBAAt(482, 367) }
	want := pixel()
	if want.B < 200 || want.R > 30 {
		t.Fatalf("sample must be on the base picture: %v", want)
	}
	for i := range 8 {
		for _, step := range []struct {
			label string
			page  page
		}{{"水印", watermark}, {"图片", pictures}} {
			var clicked bool
			for _, a := range w.AccessAll() {
				if a.Role == native.RoleButton && a.Name == step.label && a.Bounds.X < 155 {
					w.ClickAt(a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2)
					clicked = true
					break
				}
			}
			if !clicked || d.page != step.page || d.zoom != 1.3 || m.Rendering || m.Preview.Before != base || m.AfterFrame(true).Image != base || pixel() != want {
				t.Fatalf("switch %d to %s lost or reloaded the displayed bitmap: %v", i+1, step.label, pixel())
			}
		}
	}
}
