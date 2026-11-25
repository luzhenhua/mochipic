package ui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/certification"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func TestNativeCertificationEditsAndSelectedExport(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	paths := filmstripFixtures(t, 3)
	hashes := make(map[string][32]byte, len(paths))
	for _, p := range paths {
		hashes[p] = testkit.Hash(t, p)
	}
	m.Import(paths)
	wait("first original ready", func() bool { return !m.Scanning && m.OriginalReady() })
	original := m.BeforeFrame().Image
	clickWorkspace(t, w, "认证")
	wait("built-in badge images ready", func() bool {
		return d.page == certificationPage && len(m.CertificationBadges()) == certification.MaxBadges
	})
	if len(m.EditSelection) != 1 || !w.HasText("当前 1 张") {
		t.Fatal("import did not start on the first image")
	}
	control := func(role native.Role, label string) native.AccessInfo {
		t.Helper()
		e, ok := w.AccessByRole(role, label)
		if !ok {
			t.Fatalf("control %q not found", label)
		}
		return e
	}
	for _, b := range certification.Builtins() {
		if control(native.RoleToggleButton, b.Name()).Disabled {
			t.Fatalf("%s is not selectable", b.Name())
		}
	}
	checkBase := func(count int) {
		t.Helper()
		f := m.AfterFrame(true)
		if f.Image != original || len(f.Layers) != count || m.Rendering || m.PreviewErr != nil || w.HasText("无法生成预览") {
			t.Fatalf("edit replaced the original or lost its layers: layers=%d rendering=%t error=%v", len(f.Layers), m.Rendering, m.PreviewErr)
		}
	}
	checkBase(0)
	clickWorkspace(t, w, "ISO 9001")
	checkBase(1)
	clickWorkspace(t, w, "UKCA")
	checkBase(2)
	if !control(native.RoleToggleButton, "ISO 9001").Checked || !control(native.RoleToggleButton, "UKCA").Checked {
		t.Fatal("both tiles should be selected")
	}
	before := m.AfterFrame(true)
	s := control(native.RoleSlider, "间距 (px)").Bounds
	sy := s.Y + s.H/2
	w.Press(s.X+s.W*float32(m.Certification().Gap)/certification.MaxGap, sy)
	w.Move(s.X+s.W*0.23, sy)
	w.Frame()
	checkBase(2)
	if m.Certification().Gap >= 34 || m.AfterFrame(true).Layers[0].Pixels != before.Layers[0].Pixels {
		t.Fatal("spacing drag should update the existing badge layers immediately")
	}
	w.Release(s.X+s.W*0.23, sy)
	w.Frame()
	s = control(native.RoleSlider, "徽标大小 (%)").Bounds
	sy = s.Y + s.H/2
	w.Press(s.X+s.W/3, sy)
	w.Move(s.X+s.W*0.72, sy)
	w.Frame()
	checkBase(2)
	if m.Certification().Size <= 9 || m.AfterFrame(true).Layers[0].Pixels != before.Layers[0].Pixels {
		t.Fatal("size drag should update the existing badge layers immediately")
	}
	w.Release(s.X+s.W*0.72, sy)
	w.Frame()

	b := control(native.RoleButton, "移动认证徽标").Bounds
	x, y := b.X+b.W/2, b.Y+b.H/2
	w.Press(x, y)
	w.Move(x-35, y+31)
	w.Frame()
	checkBase(2)
	old, now := before.CertificationBounds, m.AfterFrame(true).CertificationBounds
	if !m.Certification().Free.Active || now.Min.Y <= old.Min.Y || m.AfterFrame(true).Layers[1].Pixels != before.Layers[1].Pixels {
		t.Fatalf("dragged group did not move together while held: %v → %v", old, now)
	}
	w.Release(x-35, y+31)
	w.Frame()
	clickWorkspace(t, w, "左下")
	if m.Certification().Free.Active || m.Certification().Anchor != 6 {
		t.Fatal("nine-grid did not snap the group")
	}
	checkBase(2)

	if err := w.ClickWith(native.Cmd, filepath.Base(paths[1])); err != nil {
		t.Fatal(err)
	}
	w.Frame()
	if m.Selected != paths[0] || !w.HasText("已选 2 张") {
		t.Fatal("Command click lost the preview or selection")
	}
	clickWorkspace(t, w, "EAC")
	checkBase(3)
	if m.Sources[1].Certification.Count() != 1 || !m.Sources[1].Certification.Has(certification.Builtins()[4]) || m.Sources[2].Certification.Count() != 0 {
		t.Fatal("adding a badge copied unrelated badges to the second image")
	}
	clickHistoryControl(t, w, native.RoleButton, filepath.Base(paths[0]))
	if len(m.EditSelection) != 1 || !m.EditSelection[paths[0]] {
		t.Fatal("clicking the focused thumbnail did not return to one photo")
	}
	clickWorkspace(t, w, "EAC")
	clickWorkspace(t, w, "UKCA")
	checkBase(1)
	custom := filepath.Join(t.TempDir(), "Test mark.png")
	testkit.PNG(t, custom, testkit.Solid(78, 39, color.NRGBA{R: 209, G: 187, B: 75, A: 255}))
	customHash := testkit.Hash(t, custom)
	m.ImportCertification(custom)
	wait("imported badge applied", func() bool { return !m.CertificationImporting && len(m.CertificationBadges()) == 11 })
	checkBase(2)
	if !control(native.RoleToggleButton, "Test mark").Checked {
		t.Fatal("imported badge was not selected on the current picture")
	}

	// The panel remains readable in each language at the minimum width.
	w.SetSize(minWindowWidth, minWindowHeight)
	for _, lang := range l10n.Languages() {
		w.SetLocale(lang.Tag)
		w.Frame()
		if !w.HasText(l10n.T(lang.Tag, "certification.builtIn")) ||
			!w.HasText(l10n.T(lang.Tag, "scope.edit")) ||
			control(native.RoleToggleButton, "ISO 45001").Bounds.W < 42 ||
			control(native.RoleButton, l10n.T(lang.Tag, "certification.import")).Bounds.W < 60 {
			t.Fatalf("truncated certification library in %s", lang.Tag)
		}
	}
	w.SetLocale(l10n.Default)
	w.Frame()

	m.SelectAll()
	opts := m.Image
	opts.Format = imaging.PNG
	m.SetImage(opts)
	out := t.TempDir()
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename})
	clickWorkspace(t, w, "导出全部 3 张")
	wait("exported three photos", func() bool { return !m.Working && m.Progress.Success == 3 })
	for i, p := range paths {
		file, err := os.Open(filepath.Join(out, filepath.Base(p)))
		if err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(file)
		file.Close()
		if err != nil || im.Bounds().Size() != image.Pt(176, 112) {
			t.Fatalf("output %d: %v", i, err)
		}
		same := true
		for y := 0; y < 112 && same; y++ {
			for x := range 176 {
				if color.NRGBAModel.Convert(im.At(x, y)) != (color.NRGBA{R: uint8(i * 37), G: 88, B: 165, A: 255}) {
					same = false
					break
				}
			}
		}
		if same != (i == 2) || hashes[p] != testkit.Hash(t, p) {
			t.Fatalf("source %d changed or wrong certification in export", i)
		}
	}
	if customHash != testkit.Hash(t, custom) {
		t.Fatal("custom badge input was modified")
	}
}

func TestNativeCertificationAllSelectedExcludesLaterImports(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	paths := filmstripFixtures(t, 3)
	dir := filepath.Dir(paths[0])
	d.importPaths([]string{dir})
	wait("imported folder", func() bool { return len(m.Sources) == 3 && !m.Scanning && m.OriginalReady() })
	clickWorkspace(t, w, "认证")
	wait("built-ins ready", func() bool { return len(m.CertificationBadges()) == certification.MaxBadges })
	clickWorkspace(t, w, "全选")
	if !w.HasText("全部 3 张") {
		t.Fatal("all three images should be selected")
	}
	base := m.AfterFrame(true).Image
	clickWorkspace(t, w, "ISO 9001")
	clickWorkspace(t, w, "UKCA")
	if f := m.AfterFrame(true); f.Image != base || m.Rendering || len(f.Layers) != 2 {
		t.Fatal("applying batch badges replaced the base image")
	}

	// The earlier edit belongs only to the original selection.
	late := filepath.Join(dir, "later.png")
	testkit.PNG(t, late, testkit.Solid(176, 112, color.NRGBA{R: 111, G: 88, B: 165, A: 255}))
	paths = append(paths, late)
	d.importPaths([]string{late})
	wait("added one more photo", func() bool { return !m.Scanning && len(m.Sources) == 4 })
	clickWorkspace(t, w, "认证")
	hashes := make(map[string][32]byte)
	for _, path := range paths {
		hashes[path] = testkit.Hash(t, path)
		clickWorkspace(t, w, filepath.Base(path))
		wait("switched preview", func() bool { return m.Selected == path && m.OriginalReady() && !m.Rendering })
		want := 2
		if path == late {
			want = 0
		}
		if f := m.AfterFrame(true); len(f.Layers) != want || m.Certification().Count() != want {
			t.Fatalf("%s did not inherit both badges", filepath.Base(path))
		}
	}

	// Add these two badges explicitly to the late photo.
	clickWorkspace(t, w, "ISO 9001")
	clickWorkspace(t, w, "UKCA")
	m.SelectAll()
	opts := m.Image
	opts.Format = imaging.PNG
	m.SetImage(opts)
	out := t.TempDir()
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename})
	clickWorkspace(t, w, "导出全部 4 张")
	wait("exported the full batch", func() bool { return !m.Working && m.Progress.Success == len(paths) })
	for i, path := range paths {
		f, err := os.Open(filepath.Join(out, filepath.Base(path)))
		if err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		base := color.NRGBA{R: uint8(i * 37), G: 88, B: 165, A: 255}
		at := func(x, y int) color.Color { return color.NRGBAModel.Convert(im.At(x, y)) }
		if at(107, 13) == base || at(157, 13) == base || at(0, 0) != base {
			t.Fatalf("export missing or misplaced badges: %s", filepath.Base(path))
		}
		if testkit.Hash(t, path) != hashes[path] {
			t.Fatalf("original changed: %s", filepath.Base(path))
		}
	}
	d.newBatch()
	if len(m.EditSelection) != 0 || m.Certification().Count() != 0 {
		t.Fatal("new batches should start in batch scope with no badges")
	}
}
