package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func TestNativeCollageSelectAutomaticallyArrangeDragAndExport(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	existing := filepath.Join(out, "COLLAGE.PNG")
	if err := os.WriteFile(existing, []byte("existing file"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldHash := testkit.Hash(t, existing)
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename})
	o := m.Image
	o.Format = imaging.PNG
	m.SetImage(o)
	colors := []color.NRGBA{{R: 220, A: 255}, {G: 210, A: 255}, {B: 200, A: 255}, {R: 230, G: 190, A: 255}}
	var paths []string
	hashes := map[string][32]byte{}
	for i, ink := range colors {
		path := filepath.Join(dir, fmt.Sprintf("%02d.png", i+1))
		testkit.PNG(t, path, testkit.Solid(90, 60, ink))
		paths = append(paths, path)
		hashes[path] = testkit.Hash(t, path)
	}
	d.importPaths(paths)
	wait("four images imported", func() bool { return d.page == pictures && !m.Scanning && len(m.Sources) == 4 })
	clickWorkspace(t, w, "图片列表")
	clickWorkspace(t, w, "全选筛选结果")
	clickWorkspace(t, w, "拼图")
	wait("collage tiles ready", func() bool { return d.page == collage && !m.Collage.Loading })
	s := &m.Collage
	if len(s.Items) != 4 || len(s.Tiles) != 4 || s.Err != nil || !w.HasText("导出拼图") {
		t.Fatalf("collage did not open: tiles=%d, err=%v, labels=%v", len(s.Tiles), s.Err, w.Texts())
	}
	g, err := s.Options.Geometry(len(s.Items))
	if err != nil || g.Columns != 2 || g.Rows != 2 {
		t.Fatalf("four files should be arranged 2 × 2: %+v, %v", g, err)
	}
	center := func(path string) (float32, float32) {
		t.Helper()
		label := l10n.Format(l10n.Default, "collage.reorderImage", l10n.Args{"file": filepath.Base(path)})
		el, ok := w.AccessByRole(native.RoleButton, label)
		if !ok {
			t.Fatalf("missing thumbnail %s", label)
		}
		return el.Bounds.X + el.Bounds.W/2, el.Bounds.Y + el.Bounds.H/2
	}
	firstTile := s.Tiles[paths[0]].Image
	assertPaint := func(ink color.NRGBA) {
		t.Helper()
		if c := w.Image().RGBAAt(350, 260); c.R != ink.R || c.G != ink.G || c.B != ink.B {
			t.Fatalf("first preview cell pixel: %v, want %v", c, ink)
		}
	}
	assertPaint(colors[0])
	x, y := center(paths[0])
	tx, ty := center(paths[3])
	w.Press(x, y)
	for i := 1; i <= 12; i++ {
		a := float32(i) / 12
		w.Move(x+(tx-x)*a, y+(ty-y)*a)
		w.Frame()
	}
	w.Release(tx, ty)
	w.Frame()
	want := []int{1, 2, 3, 0}
	for i, n := range want {
		if s.Items[i].Path != paths[n] {
			t.Fatalf("drag failed to move first tile to last: %v", s.Items)
		}
	}
	if s.Loading || s.Tiles[paths[0]].Image != firstTile {
		t.Fatal("reordering reloaded the photos")
	}
	assertPaint(colors[1])
	clickWorkspace(t, w, "导出拼图")
	wait("collage saved", func() bool { return !s.Exporting && (s.LastOutput != "" || m.Error != nil) })
	if filepath.Base(s.LastOutput) != "collage (1).png" {
		t.Fatalf("wrong output after case-insensitive conflict: %s %v", s.LastOutput, m.Error)
	}
	f, err := os.Open(s.LastOutput)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil || im.Bounds().Size() != image.Pt(228, 168) {
		t.Fatalf("collage output has wrong size: %v", err)
	}
	for i, n := range want {
		cell := g.Cells[i]
		pt := cell.Min.Add(cell.Size().Div(2))
		if got := color.NRGBAModel.Convert(im.At(pt.X, pt.Y)); got != colors[n] {
			t.Errorf("exported tile %d: got %v want %v", i, got, colors[n])
		}
	}
	for path, old := range hashes {
		if testkit.Hash(t, path) != old {
			t.Errorf("source was changed: %s", path)
		}
	}
	if testkit.Hash(t, existing) != oldHash {
		t.Fatal("existing output changed")
	}
}

func TestCustomCollageCanvasEditsAndExport(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	dir := t.TempDir()
	out := t.TempDir()
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename})
	o := m.Image
	o.Format = imaging.PNG
	m.SetImage(o)
	var paths []string
	for i, ink := range []color.NRGBA{{R: 224, A: 255}, {B: 211, A: 255}} {
		path := filepath.Join(dir, fmt.Sprintf("photo-%d.png", i))
		testkit.PNG(t, path, testkit.Solid(120, 90, ink))
		paths = append(paths, path)
	}
	d.importPaths(paths)
	wait("import", func() bool { return d.page == pictures && !m.Scanning && len(m.Sources) == 2 })
	m.SelectAll()
	w.Frame()
	clickWorkspace(t, w, "拼图")
	wait("tiles", func() bool { return len(m.Collage.Tiles) == 2 && !m.Collage.Loading })
	s := &m.Collage
	clickWorkspace(t, w, "自定义")
	w.Frame()
	if s.Options.Layout != imaging.CollageCustom {
		t.Fatal("custom layout missing")
	}
	photo := func() imaging.CollagePhoto {
		p, ok := s.Options.Photo(s.Items[0].Path)
		if !ok {
			t.Fatal("missing photo")
		}
		return p
	}
	bounds := func(label string) native.Rect {
		el, ok := w.AccessByRole(native.RoleButton, label)
		if !ok {
			t.Fatalf("control %s missing: %v", label, w.Texts())
		}
		return el.Bounds
	}
	drag := func(label string, x, y float32) {
		r := bounds(label)
		a, b := r.X+r.W/2, r.Y+r.H/2
		w.Press(a, b)
		w.Frame()
		for i := 1; i <= 8; i++ {
			w.Move(a+x*float32(i)/8, b+y*float32(i)/8)
			w.Frame()
		}
		w.Release(a+x, b+y)
		w.Frame()
	}
	label := filepath.Base(s.Items[0].Path)
	original := photo()
	tile := s.Tiles[original.Path].Image
	drag(label, 35, 3)
	moved := photo()
	if moved.Visible == original.Visible || moved.Visible.Size() != original.Visible.Size() {
		t.Fatalf("move %v -> %v", original.Visible, moved.Visible)
	}
	corner := l10n.Format(l10n.Default, "crop.handle", l10n.Args{"corner": 2})
	drag(corner, -60, 45)
	cropped := photo()
	if cropped.Visible.Min.X != moved.Visible.Min.X || cropped.Visible.Max.Y != moved.Visible.Max.Y || cropped.Visible.Dx() >= moved.Visible.Dx() || math.Abs(float64(cropped.Visible.Dx()*3-cropped.Visible.Dy()*4)) > 4 {
		t.Fatalf("crop: %v to %v", moved.Visible, cropped.Visible)
	}
	if !m.Undo() || photo() != moved || !m.Redo() || photo() != cropped {
		t.Fatal("gesture is not a single history step")
	}
	w.Frame()
	if err := w.RightClick(label); err != nil {
		t.Fatal(err)
	}
	if err := w.ChooseMenuItem("拉伸"); err != nil {
		t.Fatal(err)
	}
	w.Frame()
	if d.collage.stretch != original.Path {
		t.Fatal("stretch not activated by context menu")
	}
	// The first edge handle is on the left. A horizontal pull leaves both y coordinates fixed.
	drag("拉伸", -12, 0)
	stretched := photo()
	if stretched.Visible.Min.X >= cropped.Visible.Min.X || stretched.Visible.Min.Y != cropped.Visible.Min.Y || stretched.Visible.Max != cropped.Visible.Max {
		t.Fatalf("one-sided stretch: %v -> %v", cropped.Visible, stretched.Visible)
	}
	if !m.Undo() || photo() != cropped {
		t.Fatal("stretch undo")
	}
	w.Frame()
	if err := w.RightClick(label); err != nil {
		t.Fatal(err)
	}
	if err := w.ChooseMenuItem("结束拉伸"); err != nil {
		t.Fatal(err)
	}
	w.Frame()
	r := bounds(label)
	w.Press(r.X-10, r.Y+18)
	w.Release(r.X-10, r.Y+18)
	w.Frame()
	if d.collage.selected != "" {
		t.Fatal("blank canvas did not clear selection")
	}
	clickWorkspace(t, w, label)
	clickWorkspace(t, w, "倒角")
	if photo().Corner != imaging.CornerBevel {
		t.Fatal("bevel not applied")
	}
	if other, _ := s.Options.Photo(s.Items[1].Path); other.Corner != imaging.CornerSquare {
		t.Fatal("style leaked to other photo")
	}
	if tile != s.Tiles[original.Path].Image || s.Loading {
		t.Fatal("drag reloaded tile")
	}
	clickWorkspace(t, w, "导出拼图")
	wait("export", func() bool { return !s.Exporting && (s.LastOutput != "" || m.Error != nil) })
	if m.Error != nil {
		t.Fatal(m.Error)
	}
	f, err := os.Open(s.LastOutput)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	box := photo().Visible
	if rgb := color.NRGBAModel.Convert(im.At(box.Min.X, box.Min.Y)); rgb != (color.NRGBA{255, 255, 255, 255}) {
		t.Fatalf("bevel missing: %v", rgb)
	}
	if rgb := color.NRGBAModel.Convert(im.At(box.Min.X+box.Dx()/2, box.Min.Y+box.Dy()/2)); rgb != (color.NRGBA{R: 224, A: 255}) {
		t.Fatalf("photo center: %v", rgb)
	}
}
