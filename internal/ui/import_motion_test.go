package ui

import (
	"bytes"
	"imageflow/internal/l10n"

	"slices"
	"testing"
	"time"

	native "github.com/egoist/mygo/ui"
)

func TestWelcomeStackDragLeaveAndReducedMotion(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		dragging := false
		w := native.NewTesterWith(func(c *native.Context) { welcomeStack(c, dragging, false) }, 181, 135, native.TesterOptions{Preferences: native.Preferences{ReduceMotion: reduced}})
		w.Advance(time.Second)
		rest := slices.Clone(w.Image().Pix)
		dragging = true
		w.Frame()
		w.Advance(120 * time.Millisecond)
		middle := slices.Clone(w.Image().Pix)
		w.Advance(time.Second)
		if bytes.Equal(rest, w.Image().Pix) != reduced || bytes.Equal(middle, w.Image().Pix) != reduced {
			t.Fatalf("unexpected hover or intermediate photo position with reduced motion=%t", reduced)
		}
		dragging = false
		w.Frame()
		w.Advance(time.Second)
		if !bytes.Equal(rest, w.Image().Pix) || w.FramePending() {
			t.Fatalf("stack failed to settle back at rest with reduced motion=%t", reduced)
		}
	}
}

func TestCompactFolderSplitButton(t *testing.T) {
	d, w, _ := testWorkspace(t)
	d.newBatch()
	w.SetSize(660, 390)
	w.Frame()
	for _, lang := range l10n.Languages() {
		w.SetLocale(lang.Tag)
		w.Frame()
		pick, ok := w.AccessByRole(native.RoleButton, l10n.T(lang.Tag, "import.chooseFolder"))
		if !ok {
			t.Fatal(lang.Tag, "missing direct folder picker")
		}
		arrow, ok := w.AccessByRole(native.RoleMenuButton, l10n.T(lang.Tag, "import.folderOptions"))
		if !ok || arrow.Bounds.X-pick.Bounds.X-pick.Bounds.W > 2 || arrow.Bounds.X+arrow.Bounds.W > 660 {
			t.Fatalf("%s: split button bounds %v %v", lang.Tag, pick.Bounds, arrow.Bounds)
		}
		w.ClickAt(arrow.Bounds.X+arrow.Bounds.W/2, arrow.Bounds.Y+arrow.Bounds.H/2)
		w.Frame()
		label := l10n.T(lang.Tag, "import.recursive")
		item, ok := w.AccessByRole(native.RoleMenuItemCheckBox, label)
		if !ok || item.Bounds.X < 0 || item.Bounds.X+item.Bounds.W > 660 || (lang.Tag == "zh-Hans" && item.Bounds.W > 160) {
			t.Fatalf("%s: folder menu bounds %+v", lang.Tag, item)
		}
		before := d.model.Preferences.Recursive
		w.ClickAt(item.Bounds.X+item.Bounds.W/2, item.Bounds.Y+item.Bounds.H/2)
		w.Frame()
		if before == d.model.Preferences.Recursive {
			t.Fatal(lang.Tag, "folder option did not toggle")
		}
	}
}
