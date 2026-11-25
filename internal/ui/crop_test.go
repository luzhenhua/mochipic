package ui

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"imageflow/internal/app"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func TestNativeCropDragKeepsBaseAndExportsOnlyConfirmedBox(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	path := filepath.Join(t.TempDir(), "crop.png")
	testkit.PNG(t, path, testkit.Solid(440, 286, color.NRGBA{R: 24, G: 74, B: 205, A: 255}))
	hash := testkit.Hash(t, path)
	out := t.TempDir()
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename})
	o := m.Image
	o.Format = imaging.PNG
	m.SetImage(o)
	d.importPaths([]string{path})
	wait("original ready", m.OriginalReady)
	base := m.Preview.Before
	clickWorkspace(t, w, "裁剪与尺寸")
	if !d.cropReady() || m.Mode != app.Before || !w.HasText("完成裁剪") {
		t.Fatalf("crop editor did not open; state %+v", d.crop)
	}
	pixel := w.Image().RGBAAt(482, 367)
	if pixel.B < 190 || pixel.R > 35 {
		t.Fatalf("sample is not on the unmasked base: %v", pixel)
	}
	drag := func(fourThree bool) {
		t.Helper()
		start := d.crop.box
		label := l10n.Format(l10n.Default, "crop.handle", l10n.Args{"corner": 4})
		h, ok := w.AccessByRole(native.RoleButton, label)
		if !ok {
			t.Fatalf("crop handle is missing: %v", w.Texts())
		}
		x, y := h.Bounds.X+h.Bounds.W/2-2, h.Bounds.Y+h.Bounds.H/2-2
		w.Press(x, y)
		for i := 1; i <= 4; i++ {
			w.Move(x-float32(i)*12, y-float32(i)*8)
			w.Frame()
			if m.Rendering || m.Preview.Before != base || w.Image().RGBAAt(482, 367).B < 190 || m.SelectedSource().Crop.Active() {
				t.Fatal("drag replaced or reloaded the base or committed the draft")
			}
			if b := d.crop.box; fourThree && b.Dx()*3 != b.Dy()*4 {
				t.Fatalf("the crop lost its 4:3 ratio during the drag: %v", b)
			}
		}
		w.Release(x-48, y-32)
		w.Frame()
		if d.crop.box.Dx() >= start.Dx() || d.crop.box.Dy() >= start.Dy() {
			t.Fatalf("drag did not resize the crop: %v", d.crop.box)
		}
	}
	drag(false)
	clickWorkspace(t, w, "取消")
	if m.SelectedSource().Crop.Active() || d.cropping() {
		t.Fatal("cancel changed the source crop")
	}
	clickWorkspace(t, w, "编辑裁剪")
	clickWorkspace(t, w, "16:9")
	if got := d.crop.box.Size(); got != image.Pt(432, 243) {
		t.Fatalf("16:9 picked the wrong ratio: %v", got)
	}
	clickWorkspace(t, w, "4:3")
	if got := d.crop.box.Size(); got != image.Pt(380, 285) {
		t.Fatalf("4:3 did not fit the original: %v", got)
	}
	drag(true)
	want := d.crop.box.Size()
	clickWorkspace(t, w, "完成裁剪")
	wait("confirmed crop visible", func() bool {
		info := m.AfterFrame(true).Info
		return !m.Rendering && info.Width == want.X && info.Height == want.Y
	})
	clickWorkspace(t, w, "导出全部 1 张")
	wait("cropped file exported", func() bool { return !m.Working && m.Progress.Done() == 1 })
	_, info, err := imaging.Thumbnail(t.Context(), filepath.Join(out, "crop.png"), 64)
	if err != nil || info.Width != want.X || info.Height != want.Y || testkit.Hash(t, path) != hash {
		t.Fatalf("cropped output: %+v %v", info, err)
	}
}
