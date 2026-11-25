package ui

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"imageflow/internal/crop"
	"imageflow/internal/series"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func filmstripFixtures(t *testing.T, n int) []string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, n)
	for i := range paths {
		path := filepath.Join(dir, fmt.Sprintf("photo-%02d.png", i+1))
		testkit.PNG(t, path, testkit.Solid(176, 112, color.NRGBA{R: uint8(i * 37), G: 88, B: 165, A: 255}))
		paths[i] = path
	}
	return paths
}

func filmstripPoint(t *testing.T, w *native.Tester, path string) (float32, float32) {
	t.Helper()
	b, ok := w.AccessByRole(native.RoleButton, filepath.Base(path))
	if !ok {
		t.Fatalf("thumbnail %s is missing", path)
	}
	return b.Bounds.X + b.Bounds.W/2, b.Bounds.Y + b.Bounds.H/2
}

func TestNativeFilmstripSelectDropAndUndo(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	paths := filmstripFixtures(t, 6)
	hashes := make(map[string][32]byte, len(paths))
	for _, p := range paths {
		hashes[p] = testkit.Hash(t, p)
	}
	d.importPaths(paths)
	wait("photos and preview ready", func() bool {
		return !m.Scanning && len(m.Sources) == len(paths) && m.OriginalReady() && m.Thumbnail(paths[5]).Image != nil
	})
	base := m.Preview.Before
	commandClick := func(i int) {
		t.Helper()
		if err := w.ClickWith(native.Cmd, filepath.Base(paths[i])); err != nil {
			t.Fatal(err)
		}
		w.Frame()
	}
	commandClick(3)
	commandClick(5)
	commandClick(5)
	if d.model.EditSelection[paths[5]] || len(d.model.EditSelection) != 2 {
		t.Fatalf("Command-click did not toggle the item: %v", d.model.EditSelection)
	}
	commandClick(5)
	if m.Selected != paths[0] || m.Preview.Before != base || m.Rendering || len(d.model.EditSelection) != 3 ||
		!d.model.EditSelection[paths[0]] || !d.model.EditSelection[paths[3]] || !d.model.EditSelection[paths[5]] {
		t.Fatalf("multi-select changed the preview or selected the wrong photos: %v", d.model.EditSelection)
	}
	assertCentered := func() {
		t.Helper()
		b, ok := w.AccessByRole(native.RoleButton, "移除所选")
		axis := d.strip.tiles.X + d.strip.tiles.W/2
		if !ok || math.Abs(float64(b.Bounds.X+b.Bounds.W/2-axis)) > 1 {
			t.Fatalf("remove button is off the preview center %.1f: %+v", axis, b.Bounds)
		}
	}
	assertCentered()
	d.hideInspector = true
	w.Frame()
	assertCentered()
	d.hideInspector = false
	w.Frame()
	assertCentered()

	begin := func(i int, count int) (x, y, dx, dy float32) {
		t.Helper()
		x, y = filmstripPoint(t, w, paths[i])
		zone := d.strip.dropZone()
		dx, dy = zone.X+20, zone.Y+zone.H/2 // Away from the centered pill.
		w.Press(x, y)
		w.Move(x, y+14)
		w.Frame()
		w.Move(dx, dy)
		w.Frame()
		if !w.HasText(fmt.Sprintf("松手移除 %d 张", count)) || len(m.Sources) != len(paths) || m.Preview.Before != base {
			t.Fatalf("drag preview missing or queue changed before release: %v", w.Texts())
		}
		return
	}
	assertUnchanged := func() {
		t.Helper()
		if !slices.Equal(pathsOf(m.Sources), paths) || m.LastRemovedCount() != 0 || len(d.model.EditSelection) != 3 || m.Preview.Before != base {
			t.Fatal("a cancelled drag changed the batch, selection or preview")
		}
	}
	x, y, _, _ := begin(3, 3)
	w.Move(x, y)
	w.Release(x, y)
	w.Frame()
	assertUnchanged()
	_, _, dx, dy := begin(3, 3)
	w.Key(0, native.KeyEscape)
	w.Release(dx, dy)
	w.Frame()
	assertUnchanged()
	begin(3, 3)
	w.CancelPress()
	w.Frame()
	assertUnchanged()
	_, _, _, dy = begin(3, 3)
	w.Release(0, dy)
	w.Frame()
	assertUnchanged()

	// A drag from outside the selection removes just that photo.
	_, _, dx, dy = begin(1, 1)
	w.Release(dx, dy)
	w.Frame()
	if len(m.Sources) != 5 || slices.Contains(pathsOf(m.Sources), paths[1]) ||
		len(d.model.EditSelection) != 3 || m.Preview.Before != base {
		t.Fatal("dragging one unselected photo changed the selected group or preview")
	}
	clickWorkspace(t, w, "撤销")
	assertUnchanged()

	_, _, dx, dy = begin(3, 3)
	w.Release(dx, dy)
	w.Frame()
	want := []string{paths[1], paths[2], paths[4]}
	if !slices.Equal(pathsOf(m.Sources), want) || m.Selected != paths[1] || m.LastRemovedCount() != 3 {
		t.Fatalf("drag removed wrong photos or chose the wrong neighbor: %v", pathsOf(m.Sources))
	}
	clickWorkspace(t, w, "撤销")
	if !slices.Equal(pathsOf(m.Sources), paths) || m.Selected != paths[0] || m.LastRemovedCount() != 0 {
		t.Fatal("undo did not restore the previous order and preview")
	}
	for i, s := range m.Sources {
		if s.Ordinal != i || hashes[s.Path] != testkit.Hash(t, s.Path) {
			t.Errorf("source changed or restored out of order: %s", s.Path)
		}
	}
}

func TestNativeFilmstripKeyboardAndUndoAfterRemovingAll(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	paths := filmstripFixtures(t, 2)
	d.importPaths(paths)
	wait("two photos ready", func() bool { return !m.Scanning && len(m.Sources) == 2 && m.OriginalReady() })
	w.Key(0, native.KeyBackspace)
	w.Frame()
	if len(m.Sources) != 2 {
		t.Fatal("Backspace removed a photo without thumbnail focus")
	}
	clickWorkspace(t, w, filepath.Base(paths[1]))
	w.Key(0, native.KeyBackspace)
	w.Frame()
	if !slices.Equal(pathsOf(m.Sources), paths[:1]) {
		t.Fatal("Backspace did not remove the focused thumbnail")
	}
	w.Key(native.Cmd, native.KeyZ)
	w.Frame()
	if !slices.Equal(pathsOf(m.Sources), paths) || m.Selected != paths[1] {
		t.Fatal("Command-Z did not restore the focused photo")
	}
	if err := w.ClickWith(native.Cmd, filepath.Base(paths[0])); err != nil {
		t.Fatal(err)
	}
	w.Frame()
	clickWorkspace(t, w, "移除所选")
	if !d.emptyBatch() || d.page != importHome || !w.HasText("把图片拖到这里") {
		t.Fatal("removing the last photos did not return to the import page")
	}
	clickWorkspace(t, w, "撤销")
	if d.page != pictures || !slices.Equal(pathsOf(m.Sources), paths) || m.Selected != paths[1] {
		t.Fatal("undo on the import page did not recover the batch")
	}
}

func TestNativeFilmstripUndoKeepsSourceEdits(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	paths := filmstripFixtures(t, 2)
	book := filepath.Join(filepath.Dir(paths[0]), "book.tiff")
	data, err := os.ReadFile(filepath.Join("..", "imaging", "testdata", "different-pages.tiff"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(book, data, 0o600); err != nil {
		t.Fatal(err)
	}
	hash := testkit.Hash(t, book)
	paths = slices.Insert(paths, 1, book)
	d.importPaths(paths)
	wait("TIFF imported", func() bool { return !m.Scanning && len(m.Sources) == 3 && m.OriginalReady() })
	clickWorkspace(t, w, "book.tiff")
	if m.Selected != book || !w.Focused("book.tiff") {
		t.Fatalf("TIFF thumbnail did not keep focus when the page controls appeared: selected %q, focused %t", m.Selected, w.Focused("book.tiff"))
	}
	selection := series.Selection{Scope: series.Custom, Range: "2-3", Separate: true}
	m.SetSeriesSelection(selection)
	m.SetPreviewPart(2)
	region := crop.Region{Ratio: crop.Square, Alignment: crop.Bottom}
	m.ApplyCrop(map[string]bool{book: true}, region)
	w.Frame()
	wait("second page ready", func() bool { return m.OriginalReady() })
	w.Key(0, native.KeyBackspace)
	w.Frame()
	if m.Selected != paths[2] || slices.Contains(pathsOf(m.Sources), book) {
		t.Fatalf("the removed TIFF did not leave the queue or next image was not selected: %v, selected %q", pathsOf(m.Sources), m.Selected)
	}
	clickWorkspace(t, w, "撤销")
	wait("restored second page ready", func() bool { return m.OriginalReady() })
	s := m.SelectedSource()
	if !slices.Equal(pathsOf(m.Sources), paths) || s.Path != book || m.PreviewPart != 2 ||
		s.Crop != region || s.Selection != selection || testkit.Hash(t, book) != hash {
		t.Fatalf("undo lost TIFF source edits, order or preview page: %+v, page %d", s, m.PreviewPart)
	}
}
