package ui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func TestNativeMultiLogoDropSelectionAndBatch(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	photos := filmstripFixtures(t, 3)
	originals := map[string][32]byte{}
	for _, p := range photos {
		originals[p] = testkit.Hash(t, p)
	}
	dir := t.TempDir()
	brand, dealer := filepath.Join(dir, "brand.png"), filepath.Join(dir, "dealer.png")
	for path, c := range map[string]color.NRGBA{
		brand: {R: 240, G: 24, B: 36, A: 255}, dealer: {R: 26, G: 226, B: 70, A: 255},
	} {
		logo := testkit.Solid(100, 48, c)
		logo.SetNRGBA(0, 0, color.NRGBA{})
		testkit.PNG(t, path, logo)
		originals[path] = testkit.Hash(t, path)
	}
	d.importPaths(photos[:2])
	wait("photo ready", func() bool { return len(m.Sources) == 2 && m.OriginalReady() })
	clickWorkspace(t, w, "水印")
	clickWorkspace(t, w, "全选")
	clickWorkspace(t, w, "Logo")
	wait("editor ready", func() bool { return m.AfterFrame(true).Image != nil && !m.Rendering })
	base := m.AfterFrame(true).Image
	access := func(role native.Role, label string) native.AccessInfo {
		t.Helper()
		e, ok := w.AccessByRole(role, label)
		if !ok {
			t.Fatalf("missing %q (%v)", label, role)
		}
		return e
	}
	clickAt := func(role native.Role, label string) {
		e := access(role, label)
		w.ClickAt(e.Bounds.X+e.Bounds.W/2, e.Bounds.Y+e.Bounds.H/2)
		w.Frame()
	}
	add := access(native.RoleButton, "添加 Logo").Bounds
	if !w.DropFiles(add.X+add.W/2, add.Y+add.H/2, brand, dealer, brand) {
		t.Fatal("Logo strip rejected the file drop")
	}
	w.Frame()
	wait("two distinct layers added", func() bool { return !m.LogoPending && len(m.AfterFrame(true).Layers) == 2 })
	if d.page != watermark || len(m.Sources) != 2 || d.logoSelection != dealer {
		t.Fatalf("drop changed the batch or did not select the new layer: %d %q", len(m.Sources), d.logoSelection)
	}
	if a, b := m.Image.LogoByPath(brand), m.Image.LogoByPath(dealer); a.Anchor != 0 || b.Anchor != 8 {
		t.Fatal("new Logos should start in separate corners")
	}
	layer := func(path string) imaging.PreviewLayer {
		t.Helper()
		f := m.AfterFrame(true)
		if f.Image != base || m.Rendering || m.PreviewErr != nil {
			t.Fatalf("Logo edit changed the base or restarted rendering: %v", m.PreviewErr)
		}
		for _, l := range f.Layers {
			if l.LogoPath == path {
				return l
			}
		}
		t.Fatalf("missing layer for %s", path)
		return imaging.PreviewLayer{}
	}
	first, second := layer(brand), layer(dealer)
	clickAt(native.RoleToggleButton, "brand.png")
	if d.logoSelection != brand {
		t.Fatal("thumbnail did not select its own layer")
	}
	s := access(native.RoleSlider, "不透明度 (%)").Bounds
	y := s.Y + s.H/2
	w.Press(s.X+s.W*.82, y)
	w.Move(s.X+s.W*.51, y)
	w.Frame()
	if l := layer(brand); l.Opacity >= first.Opacity || l.Pixels != first.Pixels || layer(dealer) != second {
		t.Fatal("dragging opacity must update only the existing selected layer")
	}
	w.Release(s.X+s.W*.51, y)
	w.Frame()
	adjusted := layer(brand)

	box := access(native.RoleButton, "拖动 dealer.png").Bounds
	x, y := box.X+box.W/2, box.Y+box.H/2
	w.Press(x, y)
	w.Move(x-40, y-35)
	w.Frame()
	if moved := layer(dealer); moved.Bounds == second.Bounds || moved.Pixels != second.Pixels ||
		layer(brand) != adjusted || d.logoSelection != dealer {
		t.Fatal("canvas drag must select and move just the matching Logo")
	}
	w.Release(x-40, y-35)
	w.Frame()
	clickAt(native.RoleButton, l10n.T(l10n.Default, "anchor.bottomRight"))
	if layer(dealer).Bounds != second.Bounds {
		t.Fatal("the nine-grid did not restore the dealer's corner")
	}
	// A click on the photograph leaves the applied artwork and its pixels
	// in place while removing the outline and releasing the inspector.
	brandBox := access(native.RoleButton, "拖动 brand.png").Bounds
	dealerBox := access(native.RoleButton, "拖动 dealer.png").Bounds
	insideX, insideY := int(dealerBox.X+dealerBox.W/2), int(dealerBox.Y+dealerBox.H/2)
	edgeX, edgeY := int(dealerBox.X)+1, insideY
	selectedEdge := w.Image().RGBAAt(edgeX, edgeY)
	artwork := w.Image().RGBAAt(insideX, insideY)
	oBefore := m.Image
	photoX, photoY := (brandBox.X+dealerBox.X)/2, (brandBox.Y+dealerBox.Y)/2
	w.ClickAt(photoX, photoY)
	w.Frame()
	if access(native.RoleToggleButton, "dealer.png").Checked || access(native.RoleToggleButton, "brand.png").Checked ||
		!w.HasText(l10n.T(l10n.Default, "watermark.selectLogoHint")) {
		t.Fatal("clicking the photo should leave all Logo thumbnails unselected")
	}
	if _, ok := w.AccessByRole(native.RoleSlider, "不透明度 (%)"); ok || m.Image != oBefore ||
		layer(brand) != adjusted || layer(dealer) != second {
		t.Fatal("deselection should hide the layer's controls without changing any watermark or the base")
	}
	if im := w.Image(); im.RGBAAt(edgeX, edgeY) == selectedEdge || im.RGBAAt(insideX, insideY) != artwork {
		t.Fatal("deselection should remove the canvas outline and preserve the Logo artwork")
	}
	clickAt(native.RoleButton, "拖动 dealer.png")
	if !access(native.RoleToggleButton, "dealer.png").Checked {
		t.Fatal("clicking the Logo should select it again")
	}
	w.Advance(time.Second)
	w.ClickAt(photoX, photoY)
	w.ClickAt(photoX, photoY)
	w.Frame()
	if d.zoom != -1 || access(native.RoleToggleButton, "dealer.png").Checked {
		t.Fatal("double-click on the photograph should still zoom and leave the Logo unselected")
	}
	clickAt(native.RoleToggleButton, "dealer.png")
	d.setZoom(2)
	w.Frame()
	w.Press(photoX, photoY)
	w.Move(photoX-45, photoY-25)
	w.Release(photoX-45, photoY-25)
	w.Frame()
	if d.pan.X == 0 || !access(native.RoleToggleButton, "dealer.png").Checked || m.Image != oBefore {
		t.Fatal("panning the zoomed photo should move only the viewport")
	}
	d.setZoom(0)
	w.Frame()

	clickWorkspace(t, w, "移除 Logo")
	if m.Image.LogoByPath(dealer) != nil || len(m.AfterFrame(true).Layers) != 1 || layer(brand) != adjusted {
		t.Fatal("removal did not preserve the other layer")
	}
	bad := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(bad, []byte("not a PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	add = access(native.RoleButton, "添加 Logo").Bounds
	if !w.DropFiles(add.X+add.W/2, add.Y+add.H/2, bad, dealer) {
		t.Fatal("existing Logo strip rejected a second drop")
	}
	w.Frame()
	wait("bad file reported and valid file added", func() bool { return !m.LogoPending && m.Error != nil && len(m.AfterFrame(true).Layers) == 2 })
	if !strings.Contains(l10n.ErrorText("zh-Hans", m.Error), "bad.png") || layer(brand) != adjusted {
		t.Fatal("partial import lost existing settings or did not name the invalid file")
	}
	if d.logoSelection != dealer {
		t.Fatal("the re-added Logo was not selected")
	}
	clickWorkspace(t, w, "好")
	clickWorkspace(t, w, "图层")
	var handles []native.AccessInfo
	for _, n := range w.AccessAll() {
		if n.Role == native.RoleButton && n.Name == "拖动排序" {
			handles = append(handles, n)
		}
	}
	if len(handles) != 2 || m.AfterFrame(true).Layers[1].LogoPath != dealer {
		t.Fatal("expected the dealer layer to start above the brand layer")
	}
	x, y = handles[0].Bounds.X+handles[0].Bounds.W/2, handles[0].Bounds.Y+handles[0].Bounds.H/2
	dy := handles[1].Bounds.Y - handles[0].Bounds.Y
	w.Press(x, y)
	for i := 1; i <= 10; i++ {
		w.Move(x, y+dy*float32(i)/10)
		w.Advance(16 * time.Millisecond)
	}
	w.Release(x, y+dy)
	w.Advance(300 * time.Millisecond)
	if m.AfterFrame(true).Layers[1].LogoPath != brand || layer(brand) != adjusted {
		t.Fatal("reordering did not put the brand on top without re-rendering")
	}

	clickWorkspace(t, w, filepath.Base(photos[1]))
	wait("second photo", func() bool { return m.Selected == photos[1] && m.OriginalReady() && !m.Rendering })
	if len(m.AfterFrame(true).Layers) != 2 {
		t.Fatal("the second photo did not inherit both Logos")
	}
	d.importPaths(photos[2:])
	wait("third photo imported", func() bool { return !m.Scanning && len(m.Sources) == 3 })
	m.SelectAll()
	o := m.Image
	o.Format = imaging.PNG
	m.SetImage(o)
	out := t.TempDir()
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename})
	clickWorkspace(t, w, "导出全部 3 张")
	wait("three real outputs", func() bool { return !m.Working && m.Progress.Success == 3 })
	for i, path := range photos {
		f, err := os.Open(filepath.Join(out, filepath.Base(path)))
		if err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(f)
		f.Close()
		if err != nil || im.Bounds().Size() != image.Pt(176, 112) {
			t.Fatalf("incorrect output: %v", err)
		}
		r := color.NRGBAModel.Convert(im.At(56, 46)).(color.NRGBA)
		g := color.NRGBAModel.Convert(im.At(120, 63)).(color.NRGBA)
		if i == 2 {
			if g.G != 88 || r.G != 88 {
				t.Fatal("late import inherited a Logo")
			}
			continue
		}
		if r.R < 100 || r.G > 88 || g.G < 180 || g.R > 80 {
			t.Fatalf("output missing one of its two Logos: %v, %v", r, g)
		}
	}
	for path, want := range originals {
		if testkit.Hash(t, path) != want {
			t.Fatalf("original changed: %s", filepath.Base(path))
		}
	}
}
