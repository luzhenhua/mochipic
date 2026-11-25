package ui

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"imageflow/internal/app"
	"imageflow/internal/batch"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
)

func TestMain(m *testing.M) {
	if err := imaging.Start(); err != nil {
		panic(err)
	}
	status := m.Run()
	imaging.Shutdown()
	os.Exit(status)
}

func TestNativeViewAcceptsMixedFileDrop(t *testing.T) {
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	folder := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(filepath.Join(folder, "parts"), 0o700); err != nil {
		t.Fatal(err)
	}
	solid := testkit.Solid(31, 25, color.NRGBA{B: 210, A: 255})
	item := filepath.Join(t.TempDir(), "single.png")
	nested := filepath.Join(folder, "parts", "nested.jxl")
	testkit.PNG(t, item, solid)
	jxl, err := os.ReadFile(filepath.Join("..", "imaging", "testdata", "still.jxl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, jxl, 0o600); err != nil {
		t.Fatal(err)
	}
	updates := make(chan func(), 32)
	d := &desktop{errorSelected: map[string]bool{}, listSort: "order"}
	d.model = app.New(func(fn func()) { updates <- fn }, d.redraw)
	defer d.model.Close()
	window := native.NewTester(d.view, 1230, 760)
	click := func(s string) {
		t.Helper()
		if err := window.Click(s); err != nil {
			t.Fatal(err)
		}
		window.Frame()
	}
	assertWelcome := func() {
		t.Helper()
		if d.page != importHome || !window.HasText("选择图片…") || !window.HasText("选择文件夹…") || window.HasText("JPG") || window.HasText("PNG") || window.HasText("WebP") {
			t.Fatalf("expected the import page without format names: %v", window.Texts())
		}
		for _, name := range []string{"AVIF", "HEIC", "HEIF", "TIFF", "BMP", "ICO", "GIF", "JPEG XL", "JPEG 2000", "SVG", "PSD", "RAW"} {
			if window.HasText(name) {
				t.Fatalf("import page exposes format %q", name)
			}
		}
		for _, s := range []string{"开始处理", "切换侧栏", "切换检查器", "水印", "裁剪与尺寸", "输出", "任务结果", "预设"} {
			if _, ok := window.Access(s); ok {
				t.Fatalf("import page exposes workspace control %q", s)
			}
		}
	}
	wait := func() {
		t.Helper()
		timeout := time.After(4 * time.Second)
		for d.model.Scanning || d.model.Rendering || d.model.Planning {
			select {
			case fn := <-updates:
				fn()
				window.Frame()
			case <-timeout:
				t.Fatal("drop did not complete")
			}
		}
	}
	d.navigate(watermark)
	assertWelcome()
	click("设置")
	if !window.HasText("外观") || d.page != settings {
		t.Fatal("settings must be available before importing images")
	}
	if _, ok := window.Access("水印"); ok {
		t.Fatal("settings exposed the workspace with an empty queue")
	}
	click("返回导入")
	assertWelcome()
	if !window.DropFiles(1224, 16, t.TempDir()) {
		t.Fatal("title bar did not accept an empty folder drop")
	}
	if d.page != importHome || !window.HasText("正在扫描图片…") {
		t.Fatal("the import page must stay visible during the first scan")
	}
	wait()
	assertWelcome()
	if !window.HasText("未找到可导入的图片") {
		t.Fatal("empty folder did not get actionable feedback")
	}
	if !window.DropFiles(8, 737, item, folder, item) {
		t.Fatal("window edge did not accept the mixed file drop")
	}
	wait()
	wantSelected, err := filepath.EvalSymlinks(item)
	if err != nil {
		t.Fatal(err)
	}
	if d.page != pictures || d.model.Selected != wantSelected || len(d.model.Sources) != 2 || d.model.Scan.Duplicates != 1 || d.model.Sources[1].RelativeDir != "parts" ||
		d.model.Preview.Before == nil || d.model.PreviewErr != nil {
		t.Fatalf("import should open the workspace and preview the first image: %+v", d.model.Scan)
	}
	click("导入")
	window.Advance(time.Second)
	if !d.importMenu || !window.HasText("导入文件夹…") {
		t.Fatal("the workspace menu does not expose folder import")
	}
	window.Key(0, native.KeyEscape)
	window.Advance(time.Second)
	for _, label := range []string{"原图", "处理后", "对比"} {
		if _, found := window.AccessByRole(native.RoleButton, label); found || slices.Contains(window.Texts(), label) {
			t.Fatalf("preview mode button is still visible: %s", label)
		}
	}
	if d.model.Mode != app.Before {
		t.Fatal("the images page must show the original")
	}
	click("图片列表")
	if _, ok := window.Access("处理参数"); ok {
		t.Fatal("list unexpectedly retains the preview inspector")
	}
	click("选择 single.png")
	click("移除所选")
	wantNested, err := filepath.EvalSymlinks(nested)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(item); err != nil || len(d.model.Sources) != 1 || d.model.Sources[0].Path != wantNested {
		t.Fatalf("removal affected the original or the wrong queue entry: queue=%v, original=%v", d.model.Sources, err)
	}
	for _, p := range []page{watermark, resize, format, rename, output, tasks, presets, settings} {
		d.navigate(p)
		window.Frame()
	}
	d.navigate(presets)
	window.Frame()
	if err := window.Click("创建预设"); err != nil {
		t.Fatal(err)
	}
	if !d.presetModal {
		t.Fatal("preset editor did not open")
	}
	d.presetName = "验收预设"
	window.Frame()
	if err := window.Click("保存"); err != nil {
		t.Fatal(err)
	}
	if p, ok := d.model.Preferences.Preset(d.chosenPreset); !ok || p.Name != "验收预设" || d.presetModal {
		t.Fatalf("new preset was not saved or the dialog did not close: %v %v", p, ok)
	}
	d.navigate(pictures)
	window.Frame()
	if !d.list {
		click("图片列表")
	}
	click("清空列表")
	assertWelcome()
	if window.HasText("未找到可导入的图片") {
		t.Fatal("clearing the queue kept an old import warning")
	}
	if !window.DropFiles(1222, 740, item) {
		t.Fatal("window footer did not accept a new drop after clearing the queue")
	}
	wait()
	d.newBatch()
	window.Frame()
	assertWelcome()
}

func TestNativeWatermarkLayerDragChangesCompositingOrder(t *testing.T) {
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	d := &desktop{page: watermark, watermarkTab: 2}
	d.model = app.New(func(func()) {}, func() {})
	defer d.model.Close()
	photo := filepath.Join(t.TempDir(), "photo.png")
	testkit.PNG(t, photo, testkit.Solid(32, 24, color.NRGBA{B: 80, A: 255}))
	d.model.Sources = []files.Source{{Path: photo}}
	d.model.Selected = photo
	d.model.Image.Text.Enabled, d.model.Image.Text.Text = true, "QA"
	d.model.Image.Logo.Enabled, d.model.Image.Logo.Path = true, photo
	d.model.Image.LogoOnTop = true
	window := native.NewTester(d.view, 1230, 760)
	window.SetLocale("zh-Hans")
	var handles []native.AccessInfo
	for _, n := range window.AccessAll() {
		if n.Role == native.RoleButton && n.Name == "拖动排序" {
			handles = append(handles, n)
		}
	}
	if len(handles) != 2 {
		t.Fatalf("got %d layer handles, want two", len(handles))
	}
	x, y := handles[0].Bounds.X+handles[0].Bounds.W/2, handles[0].Bounds.Y+handles[0].Bounds.H/2
	dy := handles[1].Bounds.Y - handles[0].Bounds.Y
	window.Press(x, y)
	for i := 1; i <= 10; i++ {
		window.Move(x, y+dy*float32(i)/10)
		window.Advance(16 * time.Millisecond)
	}
	window.Release(x, y+dy)
	window.Advance(300 * time.Millisecond)
	if d.model.Image.LogoOnTop {
		t.Fatal("dragging Logo below text did not change the shared imaging option")
	}
}

func TestErrorActionsPreserveFailureRecords(t *testing.T) {
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	d := &desktop{page: failures, errorSelected: map[string]bool{}}
	d.model = app.New(func(func()) {}, func() {})
	defer d.model.Close()
	for _, name := range []string{"one.jpg", "two.jpg"} {
		src := files.Source{Path: filepath.Join(t.TempDir(), name)}
		d.model.Sources = append(d.model.Sources, src)
		d.model.Failures = append(d.model.Failures, app.Failure{Result: batch.Result{
			Entry: files.Entry{Source: src},
			Err:   errors.New("图片损坏"), Stage: "解码", Status: batch.Failed,
		}})
	}
	window := native.NewTester(d.view, 980, 630)
	for _, action := range []string{"选择 one.jpg", "跳过所选"} {
		if err := window.Click(action); err != nil {
			t.Fatal(err)
		}
		window.Frame()
	}
	if len(d.model.Failures) != 2 || d.retryableCount() != 1 || !d.skippedFailures[d.model.Failures[0].Entry.Source.Path] {
		t.Fatal("skip must preserve both failures and exclude only the selected retry")
	}
	if err := window.Click("恢复重试"); err != nil {
		t.Fatal(err)
	}
	if d.retryableCount() != 2 || len(d.model.Failures) != 2 {
		t.Fatal("restore must re-enable retries without changing failures")
	}
}
