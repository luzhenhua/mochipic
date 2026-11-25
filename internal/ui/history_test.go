package ui

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"imageflow/internal/app"
	"imageflow/internal/caption"
	"imageflow/internal/files"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func historyControl(t *testing.T, w *native.Tester, role native.Role, label string) native.AccessInfo {
	t.Helper()
	if b, ok := w.AccessByRole(role, label); ok {
		return b
	}
	t.Fatalf("missing %v %q", role, label)
	return native.AccessInfo{}
}

func clickHistoryControl(t *testing.T, w *native.Tester, role native.Role, label string) {
	t.Helper()
	b := historyControl(t, w, role, label).Bounds
	w.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	w.Frame()
}

func TestNativeHistoryRestoresBatchCaptionsAndKeepsEditorUndo(t *testing.T) {
	d, w, wait := testWorkspace(t)
	w.SetLocale("zh-Hans")
	m := d.model
	paths := filmstripFixtures(t, 3)
	d.importPaths(paths)
	wait("import", func() bool { return !m.Scanning && len(m.Sources) == 3 && m.OriginalReady() })
	initial := []caption.Caption{
		{Text: "水泵", Style: caption.Defaults()},
		{Text: "电机", Style: caption.Defaults()},
		{},
	}
	initial[1].Style.Opacity = 73
	m.SetCaption(initial[0])
	m.Select(paths[1])
	m.SetCaption(initial[1])
	m.Select(paths[0])
	clickWorkspace(t, w, "蒙版")
	wait("title shown", func() bool { return m.OriginalReady() && len(m.AfterFrame(true).Layers) == 2 })
	base := m.AfterFrame(true).Image
	undo := func() native.AccessInfo { return historyControl(t, w, native.RoleButton, "撤销") }
	redo := func() native.AccessInfo { return historyControl(t, w, native.RoleButton, "重做") }
	set := func(want []caption.Caption) {
		t.Helper()
		for i, s := range m.Sources {
			if s.Caption != want[i] {
				t.Fatalf("image %d caption got %+v, want %+v", i, s.Caption, want[i])
			}
		}
	}
	clickWorkspace(t, w, "全选")
	clickHistoryControl(t, w, native.RoleTextField, "标题")
	w.Type("整批标题")
	w.Frame()
	clickWorkspace(t, w, "图片")
	clickWorkspace(t, w, "蒙版")
	all := slices.Clone(initial)
	for i := range all {
		all[i].Text = "整批标题"
		if all[i].Style == (caption.Style{}) {
			all[i].Style = caption.Defaults()
		}
	}

	set(all)
	if undo().Disabled || !redo().Disabled || m.UndoAction() != app.HistoryCaption {
		t.Fatal("toolbar does not expose the batch action")
	}
	pos := undo().Bounds
	w.Move(pos.X+pos.W/2, pos.Y+pos.H/2)
	w.Advance(time.Second)
	if !w.HasText("撤销：编辑蒙版") {
		t.Fatal("hover did not name the whole action")
	}
	clickHistoryControl(t, w, native.RoleButton, "撤销")
	set(initial)
	if m.AfterFrame(true).Image != base || m.Rendering {
		t.Fatal("undo cleared the same base image")
	}
	clickWorkspace(t, w, "图片")
	// Exercise the same native edit-role path as the macOS Edit menu.
	w.Command("redo")
	w.Frame()
	set(all)
	clickWorkspace(t, w, "蒙版")
	wait("caption after switching tabs", func() bool { return len(m.AfterFrame(true).Layers) == 2 })
	clickHistoryControl(t, w, native.RoleButton, filepath.Base(paths[0]))
	clickHistoryControl(t, w, native.RoleTextField, "标题")
	w.Type("工业")
	w.Type("选型")
	w.Frame()
	if m.Caption().Text == all[0].Text || m.Sources[1].Caption != all[1] {
		t.Fatal("typing must only change the current title")
	}
	w.Command("undo")
	w.Frame()
	for m.Caption().Text != all[0].Text && len(m.Caption().Text) > len(all[0].Text) {
		prev := m.Caption().Text
		w.Command("undo")
		w.Frame()
		if prev == m.Caption().Text {
			break
		}
	}
	set(all)
	if m.UndoAction() != app.HistoryCaption {
		t.Fatal("text undo created an additional application step")
	}
	// Keeping the field focused must never divert a toolbar click to local undo.
	clickHistoryControl(t, w, native.RoleButton, "撤销")
	set(initial)
}

func TestNativeHistoryGroupsDragsAndKeepsRedoAfterCanceledGesture(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	paths := filmstripFixtures(t, 2)
	d.importPaths(paths)
	wait("import", func() bool { return !m.Scanning && m.OriginalReady() })
	clickWorkspace(t, w, "蒙版")
	m.SetCaption(caption.Caption{Text: "Title", Style: caption.Defaults()})
	w.Frame()
	wait("caption layer", func() bool { return len(m.AfterFrame(true).Layers) == 2 })
	b := historyControl(t, w, native.RoleSlider, "背景不透明度 (%)").Bounds
	y := b.Y + b.H/2
	from := m.Caption()
	base := m.AfterFrame(true).Image
	drag := func(to float32) {
		w.Press(b.X+b.W*.4, y)
		for _, x := range []float32{.5, .6, to} {
			w.Move(b.X+b.W*x, y)
			w.Frame()
			if m.AfterFrame(true).Image != base || m.Rendering {
				t.Fatal("opacity drag reloaded the base")
			}
		}
	}
	drag(.81)
	w.Release(b.X+b.W*.81, y)
	w.Frame()
	edited := m.Caption()
	if edited.Style.Opacity <= from.Style.Opacity {
		t.Fatal("slider did not move")
	}
	w.Key(native.Cmd, native.KeyZ)
	w.Frame()
	if m.Caption() != from || m.RedoAction() != app.HistoryCaption {
		t.Fatal("one undo did not restore the entire drag")
	}
	drag(.91)
	w.CancelPress()
	w.Frame()
	if m.Caption() != from || m.RedoAction() != app.HistoryCaption {
		t.Fatal("canceling a drag left an edit or discarded redo")
	}
	w.Key(native.Cmd|native.Shift, native.KeyZ)
	w.Frame()
	if m.Caption() != edited || m.AfterFrame(true).Image != base {
		t.Fatal("redo did not recover the drag result in place")
	}

	color := historyControl(t, w, native.RoleButton, "背景颜色").Bounds
	w.ClickAt(color.X+color.W/2, color.Y+color.H/2)
	w.Frame()
	w.Advance(time.Second)
	// The saturation panel is the largest slider in the picker.
	var area native.Rect
	for _, e := range w.AccessAll() {
		if e.Role == native.RoleSlider && e.Bounds.H > area.H {
			area = e.Bounds
		}
	}
	if area.H < 100 {
		t.Fatal("color picker did not open")
	}
	w.Press(area.X+20, area.Y+20)
	w.Move(area.X+area.W*.42, area.Y+52)
	w.Frame()
	w.Move(area.X+area.W*.76, area.Y+81)
	w.Frame()
	w.Release(area.X+area.W*.76, area.Y+81)
	w.Frame()
	if m.Caption().Style.Background == edited.Style.Background {
		t.Fatal("the color picker did not change the background")
	}
	w.Key(0, native.KeyEscape)
	w.Frame()
	clickHistoryControl(t, w, native.RoleButton, "撤销")
	if m.Caption() != edited {
		t.Fatalf("one undo did not restore the whole color gesture: got %+v want %+v", m.Caption(), edited)
	}
}

func TestNativeHistoryReopensCollageAndRestoresWholeQueue(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	paths := filmstripFixtures(t, 3)
	hashes := make([][32]byte, len(paths))
	for i, p := range paths {
		hashes[i] = testkit.Hash(t, p)
	}
	d.importPaths(paths)
	wait("import", func() bool { return !m.Scanning && m.OriginalReady() })
	m.SelectAll()
	d.list = true
	w.Frame()
	clickWorkspace(t, w, "拼图")
	wait("collage tiles", func() bool { return m.HasCollage() && !m.Collage.Loading })
	before := slices.Clone(m.Collage.Items)
	m.MoveCollage(paths[0], paths[2])
	after := slices.Clone(m.Collage.Items)
	d.leaveCollage()
	w.Frame()
	w.Key(native.Cmd, native.KeyZ)
	w.Frame()
	if !slices.Equal(m.Collage.Items, before) || d.page != pictures {
		t.Fatal("collage undo failed after leaving its page")
	}
	clickWorkspace(t, w, "拼图")
	w.Key(native.Cmd|native.Shift, native.KeyZ)
	w.Frame()
	if !slices.Equal(m.Collage.Items, after) || m.Collage.Loading {
		t.Fatal("redo reset collage order or reloaded the tiles")
	}
	d.leaveCollage()
	d.removeQueueSources(d.model.EditSelection)
	w.Frame()
	if d.page != importHome || len(m.Sources) != 0 || m.HasCollage() {
		t.Fatal("removing all photos should suspend the collage")
	}
	clickHistoryControl(t, w, native.RoleButton, "撤销")
	if d.page != pictures || !slices.Equal(pathsOf(m.Sources), paths) || !slices.Equal(m.Collage.Items, after) {
		t.Fatal("undo at import did not restore queue and collage")
	}
	output := files.Settings{Directory: t.TempDir(), Conflict: files.Skip, KeepTree: true}
	m.SetOutput(output)
	w.Frame()
	if !historyControl(t, w, native.RoleButton, "重做").Disabled {
		t.Fatal("new edit after undo did not clear redo")
	}
	for i, p := range paths {
		if testkit.Hash(t, p) != hashes[i] {
			t.Fatal("history changed an original file")
		}
	}
	d.newBatch()
	w.Frame()
	if _, visible := w.AccessByRole(native.RoleButton, "撤销"); visible || m.UndoAction() != "" || m.HasCollage() {
		t.Fatal("explicit new batch carried over history")
	}
}
