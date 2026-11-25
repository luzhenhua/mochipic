package ui

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	native "github.com/egoist/mygo/ui"
	"imageflow/internal/app"
	"imageflow/internal/caption"
	"imageflow/internal/crop"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/testkit"
)

func TestNativeFourPhotoEditScopeAndActualExports(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	out := t.TempDir()
	m.SetOutput(files.Settings{Directory: out, Conflict: files.Rename})
	defaults := imaging.Defaults()
	defaults.Format = imaging.JPEG
	m.SetImage(defaults)
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	var paths []string
	var hashes [][32]byte
	for i, name := range []string{"01-pump", "02-motor", "03-valve", "04-shop"} {
		path := filepath.Join(dir, name+".png")
		testkit.PNG(t, path, testkit.Solid(984, 656, color.NRGBA{R: uint8(150 + i*10), G: 177, B: 191, A: 255}))
		paths = append(paths, path)
		hashes = append(hashes, testkit.Hash(t, path))
	}
	d.importPaths(paths)
	wait("first photo", func() bool { return !m.Scanning && m.OriginalReady() && len(m.Sources) == 4 })
	clickWorkspace(t, w, "裁剪与尺寸")
	clickWorkspace(t, w, "4:3")
	region := crop.Region{Reference: d.crop.size, Box: d.crop.box, Ratio: crop.FourThree}
	clickWorkspace(t, w, "完成裁剪")
	if !w.HasText("当前 1 张") || m.Sources[0].Crop != region || m.Sources[1].Crop.Active() {
		t.Fatal("crop escaped the first photo")
	}
	thumb := func(i int) { clickHistoryControl(t, w, native.RoleButton, filepath.Base(paths[i])) }
	clickWorkspace(t, w, "水印")
	thumb(2)
	opt := m.Image
	opt.Text.Size = 72
	m.SetImage(opt)
	thumb(1)
	if err := w.ClickWith(native.Cmd, filepath.Base(paths[2])); err != nil {
		t.Fatal(err)
	}
	w.Frame()
	if !d.hasMixed("watermark.fontSize") || !w.HasText("已选 2 张") {
		t.Fatal("different font sizes were not disclosed")
	}
	clickHistoryControl(t, w, native.RoleTextField, "字号 (px)")
	w.Type("48")
	w.Key(0, native.KeyEnter)
	w.Frame()
	if d.hasMixed("watermark.fontSize") || m.PhotoSettings(m.Sources[2]).Image.Text.Size != 48 {
		t.Fatal("entering the focused photo's size did not unify both photos")
	}
	clickHistoryControl(t, w, native.RoleButton, "撤销")
	if !d.hasMixed("watermark.fontSize") || m.PhotoSettings(m.Sources[2]).Image.Text.Size != 72 {
		t.Fatal("undo failed to restore two different sizes")
	}
	clickHistoryControl(t, w, native.RoleTextField, "文字内容")
	w.Type("技术")
	w.Type("样本")
	w.Frame()
	if m.PhotoSettings(m.Sources[2]).Image.Text.Size != 72 || m.PhotoSettings(m.Sources[1]).Image.Text.Size != 48 {
		t.Fatal("typing replaced a different font size")
	}
	clickHistoryControl(t, w, native.RoleButton, "撤销")
	if m.PhotoSettings(m.Sources[1]).Image.Text.Text != "" || m.PhotoSettings(m.Sources[2]).Image.Text.Text != "" {
		t.Fatal("one undo did not recover both images")
	}
	clickHistoryControl(t, w, native.RoleButton, "重做")
	clickWorkspace(t, w, "格式")
	if !w.HasText("已选 2 张") {
		t.Fatal("page navigation lost membership")
	}
	thumb(1)
	w.Key(native.Cmd, native.KeyA)
	w.Frame()
	if len(m.EditSelection) != 4 || !w.HasText("全部 4 张") {
		t.Fatal("Command-A in the filmstrip did not select the batch")
	}
	clickWorkspace(t, w, "WebP")
	if m.UndoAction() != app.HistoryFormat {
		t.Fatal("bulk format change missing from history")
	}
	thumb(3)
	clickWorkspace(t, w, "蒙版")
	clickHistoryControl(t, w, native.RoleTextField, "标题")
	const title = "立式管道泵 · 出厂实拍"
	w.Type(title)
	w.Frame()
	w.Key(native.Cmd, native.KeyA)
	w.Frame()
	if len(m.EditSelection) != 1 || m.Selected != paths[3] {
		t.Fatal("select-all in the text field changed the photo selection")
	}
	clickWorkspace(t, w, "重命名")
	clickWorkspace(t, w, "模板")
	clickHistoryControl(t, w, native.RoleTextField, "命名模板")
	w.Key(native.Cmd, native.KeyA)
	w.Type("shop-final")
	w.Frame()
	clickWorkspace(t, w, "输出")
	wait("whole-batch output list", func() bool { return !m.Planning && len(m.Plan.Entries) == 4 && m.Thumbnail(paths[0]).Image != nil })
	if !w.HasText("全部 4 张") || !w.HasText("shop-final.webp") || !w.HasText(title) || !w.HasText("技术样本") {
		t.Fatalf("missing per-photo output summary: %v", w.Texts())
	}
	clickWorkspace(t, w, "导出全部 4 张")
	wait("actual WebP exports", func() bool { return !m.Working && m.Progress.Done() == 4 })
	if m.Progress.Success != 4 {
		t.Fatalf("export: %+v", m.Report)
	}
	for i, source := range paths {
		want := defaults
		want.Format = imaging.WebP
		switch i {
		case 0:
			want.Crop = region
		case 1, 2:
			want.Text.Text, want.Text.Enabled = "技术样本", true
			if i == 2 {
				want.Text.Size = 72
			}
		case 3:
			want.Caption = caption.Caption{Text: title, Style: caption.Defaults()}
		}
		name := []string{"01-pump.webp", "02-motor.webp", "03-valve.webp", "shop-final.webp"}[i]
		actual, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		var expected bytes.Buffer
		info, err := imaging.Export(t.Context(), source, want, &expected)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, expected.Bytes()) || testkit.Hash(t, source) != hashes[i] {
			t.Fatalf("wrong pixels/metadata or source hash for %s", name)
		}
		size := image.Pt(984, 656)
		if i == 0 {
			size = region.Box.Size()
		}
		if image.Pt(info.Width, info.Height) != size {
			t.Fatalf("wrong dimensions for %s: %+v", name, info)
		}
	}
}

func TestScopedLogoImportAndRetryKeepTheOriginalPhotoSettings(t *testing.T) {
	d, _, wait := testWorkspace(t)
	m := d.model
	photos := filmstripFixtures(t, 2)
	hashes := [][32]byte{testkit.Hash(t, photos[0]), testkit.Hash(t, photos[1])}
	d.importPaths(photos)
	wait("import", func() bool { return !m.Scanning && len(m.Sources) == 2 && m.OriginalReady() })
	output := t.TempDir()
	m.SetOutput(files.Settings{Directory: output, Conflict: files.Rename})
	m.Select(photos[1])
	logo := filepath.Join(t.TempDir(), "brand.png")
	art := testkit.Solid(36, 24, color.NRGBA{R: 250, G: 75, B: 8, A: 170})
	testkit.PNG(t, logo, art)
	validLogo, err := os.ReadFile(logo)
	if err != nil {
		t.Fatal(err)
	}
	m.AddLogos([]string{logo})
	m.Select(photos[0]) // Change focus before the validation callback is delivered.
	wait("logo validation", func() bool { return !m.LogoPending })
	second := m.PhotoSettings(m.Sources[1]).Image
	if len(m.Image.LogoMarks()) != 0 || second.LogoByPath(logo) == nil {
		t.Fatal("the imported Logo followed the preview to a different photo")
	}
	m.Select(photos[1])
	opt := m.Image
	opt.Format = imaging.PNG
	m.SetImage(opt)
	r := m.Naming
	r.KeepOriginal, r.Template = false, "original-plan"
	m.SetNaming(r)
	// Replace only the generated test artwork to force a recoverable decode failure.
	if err := os.WriteFile(logo, []byte("incomplete PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Select(photos[0])
	if err := m.Start(nil); err != nil {
		t.Fatal(err)
	}
	wait("one success and one failure", func() bool { return !m.Working && m.Progress.Done() == 2 })
	failed := m.Report.FailedSources()
	if len(failed) != 1 || failed[0].Path != photos[1] || m.Progress.Success != 1 {
		t.Fatalf("unexpected failures: %+v", m.Report)
	}
	m.Select(photos[1])
	updated := opt
	updated.Format, updated.Logo.Path = imaging.JPEG, ""
	m.SetImage(updated)
	r.Template = "later-edits"
	m.SetNaming(r)
	if err := os.WriteFile(logo, validLogo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(failed); err != nil {
		t.Fatal(err)
	}
	wait("retry", func() bool { return !m.Working && m.Progress.Success == 1 })
	actual, err := os.ReadFile(filepath.Join(output, "original-plan.png"))
	if err != nil {
		t.Fatal(err)
	}
	var expected bytes.Buffer
	if _, err := imaging.Export(t.Context(), photos[1], opt, &expected); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected.Bytes()) || len(m.Failures) != 0 || m.Naming.Template != "later-edits" {
		t.Fatal("retry lost the original name, encoding or logo, or replaced the newer edits")
	}
	for i, path := range photos {
		if testkit.Hash(t, path) != hashes[i] {
			t.Fatal("retry modified the input photo")
		}
	}
}
