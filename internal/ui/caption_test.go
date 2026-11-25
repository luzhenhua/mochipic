package ui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/config"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func TestNativeCaptionEditingBatchStyleAndExport(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	photoDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bg := color.NRGBA{R: 170, G: 169, B: 168, A: 255}
	photos, sums := make([]string, 3), make([][32]byte, 3)
	for i, name := range []string{"01-pump.png", "02-motor.png", "03-valve.png"} {
		photos[i] = filepath.Join(photoDir, name)
		testkit.PNG(t, photos[i], testkit.Solid(640, 420, bg))
		sums[i] = testkit.Hash(t, photos[i])
	}
	d.importPaths(photos[:2])
	w.SetLocale("zh-Hans")
	w.Frame()
	wait("initial photos", func() bool { return !m.Scanning && m.OriginalReady() })
	clickWorkspace(t, w, "蒙版")
	wait("empty mask", func() bool { return d.page == captionPage && !m.Rendering && m.AfterFrame(true).Image != nil })
	base := m.AfterFrame(true).Image
	layers := func() []imaging.PreviewLayer {
		t.Helper()
		f := m.AfterFrame(true)
		if f.Image != base || m.Rendering || m.PreviewErr != nil {
			t.Fatalf("caption editing replaced the base or blocked preview: %v", m.PreviewErr)
		}
		return f.Layers
	}
	find := func(role native.Role, label string) native.AccessInfo {
		t.Helper()
		el, ok := w.AccessByRole(role, label)
		if !ok {
			t.Fatalf("missing %q (%v)", label, role)
		}
		return el
	}
	click := func(role native.Role, label string) {
		t.Helper()
		r := find(role, label).Bounds
		w.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		w.Frame()
	}
	if len(layers()) != 0 || m.Caption().Style.Opacity != 40 {
		t.Fatal("default controls must be editable with no strip in the image")
	}
	const title = "Industrial Pump Guide\nPressure & flow"
	click(native.RoleTextField, "标题")
	w.Type(title)
	w.Frame()
	wait("wrapped title", func() bool { return len(m.AfterFrame(true).Layers) == 2 })
	if m.Caption().Text != title || m.Sources[1].Caption.Active() {
		t.Fatalf("title or default scope: %q %+v", m.Caption().Text, m.Sources[1].Caption)
	}
	first := layers()
	slider := find(native.RoleSlider, "背景不透明度 (%)").Bounds
	y := slider.Y + slider.H/2
	x := slider.X + slider.W*.63
	w.Press(slider.X+slider.W*.4, y)
	w.Move(x, y)
	w.Frame()
	if got := layers(); got[0].Fill.A <= first[0].Fill.A || got[1] != first[1] {
		t.Fatal("opacity drag must update the fill and reuse the existing glyphs")
	}
	w.Release(x, y)
	w.Frame()
	click(native.RoleTextField, "标题")
	w.Type(" Q5")
	w.Frame()
	if m.Caption().Text != title+" Q5" || layers()[1] != first[1] {
		t.Fatal("typing should retain the current title artwork while the new layout is pending")
	}
	wait("replacement glyphs", func() bool { return layers()[1].Pixels != first[1].Pixels })
	click(native.RoleToggleButton, "更多设置")
	if _, ok := w.AccessByRole(native.RoleSlider, "字号（占图宽 %）"); !ok {
		t.Fatal("advanced controls did not open")
	}
	click(native.RoleToggleButton, "更多设置")
	style := m.Caption().Style
	click(native.RoleButton, "保存样式")
	wait("style saved", func() bool { return !m.PrefsPending && m.Preferences.CaptionStyle == style })
	prefPath, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	p, err := config.Load(prefPath)
	if err != nil || p.CaptionStyle != style {
		t.Fatalf("style was not written to disk: %v", err)
	}
	clickWorkspace(t, w, "02-motor.png")
	wait("second photo", func() bool { return m.Selected == photos[1] && m.OriginalReady() && !m.Rendering })
	if m.Caption().Text != "" || m.Caption().Style != style || len(m.AfterFrame(true).Layers) != 0 {
		t.Fatal("switching photos leaked the first title or lost the default style")
	}
	clickWorkspace(t, w, "01-pump.png")
	wait("first caption restored", func() bool {
		return m.Selected == photos[0] && m.OriginalReady() && len(m.AfterFrame(true).Layers) == 2
	})
	clickWorkspace(t, w, "全选")
	click(native.RoleTextField, "标题")
	w.Type(title + " Q5")
	w.Frame()
	if m.Sources[1].Caption != m.Sources[0].Caption {
		t.Fatal("batch apply did not copy both title and style")
	}
	clickHistoryControl(t, w, native.RoleButton, "01-pump.png")
	click(native.RoleTextField, "标题")
	w.Key(native.Cmd, native.KeyA)
	w.Key(0, native.KeyBackspace)
	w.Frame()
	if len(m.AfterFrame(true).Layers) != 0 || m.Caption().Text != "" || !m.Sources[1].Caption.Active() {
		t.Fatal("clearing the title must remove only the current title strip")
	}
	d.importPaths(photos[2:])
	wait("one later import", func() bool { return !m.Scanning && len(m.Sources) == 3 })
	if m.Sources[2].Caption.Active() {
		t.Fatal("a later import must not inherit the batch's title")
	}
	output := t.TempDir()
	m.SelectAll()
	opts := m.Image
	opts.Format = imaging.PNG
	m.SetImage(opts)
	m.SetOutput(files.Settings{Directory: output, Conflict: files.Rename})
	clickWorkspace(t, w, "导出全部 3 张")
	wait("three processed images", func() bool { return !m.Working && m.Progress.Done() == 3 })
	if m.Progress.Success != 3 {
		t.Fatalf("export: %+v", m.Report)
	}
	readPixel := func(im image.Image, x, y int) color.NRGBA {
		return color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
	}
	for i, path := range photos {
		f, err := os.Open(filepath.Join(output, filepath.Base(path)))
		if err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(f)
		f.Close()
		if err != nil || readPixel(im, 0, 0) != bg || (readPixel(im, 0, 410) == bg) != (i != 1) {
			t.Fatalf("wrong caption coverage for image %d: %v", i+1, err)
		}
		if testkit.Hash(t, path) != sums[i] {
			t.Fatal("original file changed")
		}
	}
}
