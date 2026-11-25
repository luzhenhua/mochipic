package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/files"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func planControl(t *testing.T, w *native.Tester, role native.Role, label string) native.AccessInfo {
	t.Helper()
	e, ok := w.AccessByRole(role, label)
	if !ok {
		t.Fatalf("missing %q in the native view", label)
	}
	return e
}

func TestNativeRenameEditsKeepPreviewRowsAndScroll(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "QA-photo-17.webp"), []byte("existing file"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename})
	paths := filmstripFixtures(t, namePerPage*2)
	d.importPaths(paths)
	wait("rename input ready", func() bool {
		return !m.Scanning && !m.Planning && m.PlanErr == nil && len(m.Plan.Entries) == len(paths) && m.OriginalReady()
	})
	clickWorkspace(t, w, "重命名")
	clickWorkspace(t, w, "全选")
	clickWorkspace(t, w, "下一页")
	photo := paths[16]
	rowLabel := filepath.Base(photo)
	wait("thumbnail on page two", func() bool { return m.Thumbnail(photo).Image != nil })
	start := planControl(t, w, native.RoleButton, rowLabel)
	w.Scroll(400, 430, 0, 138)
	w.Frame()
	row := planControl(t, w, native.RoleButton, rowLabel)
	if row.Offscreen || row.Bounds.Y >= start.Bounds.Y {
		t.Fatalf("the second page did not scroll: %v → %v", start.Bounds, row.Bounds)
	}
	thumb, preview := m.Thumbnail(photo).Image, m.Preview.Before
	assertRows := func(name string) {
		t.Helper()
		e := planControl(t, w, native.RoleButton, rowLabel)
		if d.namePage != 1 || e.Bounds != row.Bounds || !w.HasText(name) || !w.HasText("输出文件名") ||
			m.Thumbnail(photo).Image != thumb || m.Preview.Before != preview || m.Rendering {
			t.Fatalf("editing replaced the preview, scrolled the list or hid %q", name)
		}
		if m.Planning {
			if !planControl(t, w, native.RoleButton, "复制命名预览").Disabled {
				t.Fatal("the prior snapshot should not be copyable while a new plan is pending")
			}
			if _, ok := m.CurrentEntry(); ok {
				t.Fatal("the prior snapshot was exposed as a current plan")
			}
		}
	}
	const original, updated = "photo-17.webp", "QA-photo-17 (1).webp"
	clickWorkspace(t, w, "模板")
	if !m.Planning || m.Naming.KeepOriginal {
		t.Fatal("switching to the template did not schedule a new plan")
	}
	assertRows(original)
	field := planControl(t, w, native.RoleTextField, "命名模板")
	w.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
	w.Key(native.Cmd, native.KeyA)
	w.Type("QA-{")
	w.Frame()
	assertRows(original)
	w.Type("original}")
	w.Frame()
	assertRows(original)
	if m.Naming.Template != "QA-{original}" || !w.Focused("命名模板") {
		t.Fatalf("editing lost the template or focus: %q", m.Naming.Template)
	}
	wait("latest naming plan resolves the existing file collision", func() bool {
		return !m.Planning && len(m.Plan.Entries) == len(paths) && m.Plan.Entries[16].Relative == updated
	})
	assertRows(updated)
	clickHistoryControl(t, w, native.RoleButton, "撤销")
	assertRows(updated)
	wait("original template restored in the scrolled list", func() bool {
		return !m.Planning && !m.Naming.KeepOriginal && m.Naming.Template == ""
	})
	clickHistoryControl(t, w, native.RoleButton, "重做")
	wait("redo restored the edited naming plan", func() bool {
		return !m.Planning && m.Plan.Entries[16].Relative == updated
	})
	assertRows(updated)
	clickWorkspace(t, w, "保留原名")
	assertRows(updated)
	wait("original naming plan restored", func() bool { return !m.Planning })
	assertRows(original)
}

func TestNativeOutputSettingsKeepTreeUntilNewPlanIsReady(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	in, out := t.TempDir(), t.TempDir()
	for _, dir := range []string{in, out} {
		if err := os.Mkdir(filepath.Join(dir, "parts"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	photo := filepath.Join(in, "parts", "clip.png")
	testkit.PNG(t, photo, testkit.Solid(140, 90, color.NRGBA{R: 30, G: 90, B: 160, A: 255}))
	existing := filepath.Join(out, "parts", "clip.webp")
	if err := os.WriteFile(existing, []byte("existing file"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoHash, existingHash := testkit.Hash(t, photo), testkit.Hash(t, existing)
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename, KeepTree: true})
	d.importPaths([]string{in})
	wait("output plan ready", func() bool {
		return !m.Scanning && !m.Planning && m.PlanErr == nil && len(m.Plan.Entries) == 1
	})
	clickWorkspace(t, w, "输出")
	originalFolder := planControl(t, w, native.RoleButton, "折叠 parts").Bounds
	planControl(t, w, native.RoleButton, "clip (1).webp")
	assertTree := func(file string) {
		t.Helper()
		if !m.Planning || !w.HasText("目录预览") ||
			planControl(t, w, native.RoleButton, "折叠 parts").Bounds != originalFolder {
			t.Fatal("pending output settings replaced the directory preview")
		}
		planControl(t, w, native.RoleButton, file)
	}
	clickWorkspace(t, w, "跳过已有文件")
	assertTree("clip (1).webp")
	wait("skip decision applied", func() bool {
		return !m.Planning && len(m.Plan.Entries) == 1 && m.Plan.Entries[0].Skip
	})
	planControl(t, w, native.RoleButton, "clip.webp")
	toggleTree := func() {
		t.Helper()
		s := planControl(t, w, native.RoleCheckBox, "保留目录结构").Bounds
		w.ClickAt(s.X+s.W/2, s.Y+s.H/2)
		w.Frame()
	}
	toggleTree()
	assertTree("clip.webp")
	wait("flat plan ready", func() bool {
		return !m.Planning && len(m.Plan.Entries) == 1 && m.Plan.Entries[0].Relative == "clip.webp" && !m.Plan.Entries[0].Skip
	})
	if _, ok := w.AccessByRole(native.RoleButton, "折叠 parts"); ok {
		t.Fatal("the new plan kept a folder after flattening")
	}
	flat := planControl(t, w, native.RoleButton, "clip.webp").Bounds
	w.Key(native.Cmd, native.KeyZ)
	w.Frame()
	if !m.Planning || !m.Output.KeepTree || planControl(t, w, native.RoleButton, "clip.webp").Bounds != flat {
		t.Fatal("the old flat preview disappeared while restoring the folder structure")
	}
	clickWorkspace(t, w, "导出全部 1 张") // Start before the background preview settles.
	wait("latest output settings used for export", func() bool { return !m.Working && m.Progress.Done() == 1 })
	if m.Progress.Skipped != 1 || testkit.Hash(t, existing) != existingHash || testkit.Hash(t, photo) != photoHash {
		t.Fatal("processing used the stale flat preview or modified a protected file")
	}
	if _, err := os.Stat(filepath.Join(out, "clip.webp")); !os.IsNotExist(err) {
		t.Fatalf("processing wrote an output from the stale plan: %v", err)
	}
}
