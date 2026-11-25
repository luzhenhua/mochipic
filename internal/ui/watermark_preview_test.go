package ui

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"
	"time"

	"imageflow/internal/app"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/testkit"

	native "github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func TestNativeWatermarkDraftAndLayerEditing(t *testing.T) {
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	photo, logo := filepath.Join(dir, "photo.png"), filepath.Join(dir, "logo.png")
	testkit.PNG(t, photo, testkit.Solid(412, 256, color.NRGBA{R: 240, G: 235, B: 225, A: 255}))
	logoImage := testkit.Solid(52, 22, color.NRGBA{R: 220, B: 30, A: 255})
	logoImage.SetNRGBA(0, 0, color.NRGBA{})
	testkit.PNG(t, logo, logoImage)
	photoHash, logoHash := testkit.Hash(t, photo), testkit.Hash(t, logo)

	updates := make(chan func(), 128)
	d := &desktop{errorSelected: map[string]bool{}}
	m := app.New(func(fn func()) { updates <- fn }, d.redraw)
	d.model = m
	defer m.Close()
	window := native.NewTester(d.view, 1310, 810)
	d.importPaths([]string{photo})
	window.Frame()

	wait := func(reason string, cond func() bool) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for !cond() {
			select {
			case fn := <-updates:
				fn()
				window.Frame()
			case <-deadline:
				t.Fatalf("%s: %v (rendering %t)", reason, m.PreviewErr, m.Rendering)
			}
		}
	}
	click := func(label string) {
		t.Helper()
		if err := window.Click(label); err != nil {
			t.Fatal(err)
		}
		window.Frame()
	}
	control := func(role native.Role, label string) native.AccessInfo {
		t.Helper()
		a, ok := window.AccessByRole(role, label)
		if !ok {
			t.Fatalf("missing control %q", label)
		}
		return a
	}
	clickControl := func(role native.Role, label string) {
		a := control(role, label)
		window.ClickAt(a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2)
		window.Frame()
	}
	wait("import preview", func() bool { return !m.Scanning && m.Preview.Before != nil })
	d.navigate(watermark)
	window.Frame()
	wait("initial preview", func() bool { return m.AfterFrame(true).Image != nil && !m.Rendering })
	base := m.AfterFrame(true).Image
	checkBase := func(stage string, n int) {
		t.Helper()
		f := m.AfterFrame(true)
		if f.Image != base || len(f.Layers) != n || m.PreviewErr != nil {
			t.Fatalf("%s changed or lost the base: layers=%d error=%v", stage, len(f.Layers), m.PreviewErr)
		}
		if window.HasText("无法生成预览") {
			t.Fatalf("%s replaced the canvas with an error", stage)
		}
	}
	dragOpacity := func(index int) {
		t.Helper()
		layer := m.AfterFrame(true).Layers[index]
		slider := control(native.RoleSlider, "不透明度 (%)")
		b := slider.Bounds
		x, y := b.X+b.W*float32(layer.Opacity)/100, b.Y+b.H/2
		window.Press(x, y)
		window.Move(b.X+b.W*0.29, y)
		window.Frame()
		f := m.AfterFrame(true)
		if f.Image != base || len(f.Layers) <= index {
			t.Fatal("opacity drag replaced the base or hid the layer")
		}
		got := f.Layers[index]
		if got.Pixels != layer.Pixels || got.Opacity <= 0 || got.Opacity >= layer.Opacity {
			t.Fatalf("opacity drag must update the existing layer while held: %d → %d", layer.Opacity, got.Opacity)
		}
		window.Release(b.X+b.W*0.29, y)
		window.Frame()
	}

	noSwitch := func(label string) {
		t.Helper()
		if _, ok := window.AccessByRole(native.RoleSwitch, label); ok {
			t.Fatalf("watermark still has an enable switch: %s", label)
		}
	}
	noSwitch("文字水印")
	checkBase("empty text", 0)
	control(native.RoleSlider, "不透明度 (%)")
	clickControl(native.RoleButton, l10n.T(l10n.Default, "anchor.center"))
	if m.Image.Text.Anchor != 4 || m.Image.Text.Enabled {
		t.Fatal("empty text should allow changing its placement without applying a layer")
	}
	checkBase("configure empty text", 0)
	clickControl(native.RoleTextField, "文字内容")
	window.Type(" 　")
	window.Frame()
	if m.Image.Text.Enabled || m.Image.MissingWatermark() != nil {
		t.Fatal("whitespace should not activate a layer or prevent export")
	}
	window.Type("MochiPic")
	window.Frame()
	wait("typed text layer", func() bool { return len(m.AfterFrame(true).Layers) == 1 })
	checkBase("type text", 1)
	if m.Image.Text.Anchor != 4 {
		t.Fatal("typing text lost the placement set before adding content")
	}
	dragOpacity(0)
	oldText := m.AfterFrame(true).Layers[0].Pixels
	clickControl(native.RoleTextField, "文字内容")
	window.Type(" QA")
	window.Frame()
	wait("changed glyphs", func() bool {
		layers := m.AfterFrame(true).Layers
		return len(layers) == 1 && layers[0].Pixels != oldText
	})
	checkBase("edit text", 1)

	click("Logo")
	noSwitch("Logo 水印")
	checkBase("empty Logo chooser", 1)
	control(native.RoleSlider, "宽度比例 (%)")
	clickControl(native.RoleCheckBox, "保持宽高比")
	control(native.RoleSlider, "高度比例 (%)")
	if m.Image.Logo.KeepRatio || m.Image.Logo.Enabled {
		t.Fatal("empty Logo should allow changing sizing without applying a layer")
	}
	checkBase("configure empty Logo", 1)
	chooser := control(native.RoleButton, "添加 Logo")
	if !window.DropFiles(chooser.Bounds.X+chooser.Bounds.W/2, chooser.Bounds.Y+chooser.Bounds.H/2, logo) {
		t.Fatal("Logo selector did not receive the PNG drop")
	}
	window.Frame()
	wait("decoded Logo layer", func() bool { return len(m.AfterFrame(true).Layers) == 2 })
	if !m.Image.Logo.Enabled || m.Image.Logo.KeepRatio || len(m.Sources) != 1 {
		t.Fatal("adding a Logo did not apply its configured sizing to the current image")
	}
	checkBase("choose a transparent Logo", 2)
	dragOpacity(1)
	layer := func(kind imaging.WatermarkKind) imaging.PreviewLayer {
		t.Helper()
		for _, layer := range m.AfterFrame(true).Layers {
			if layer.Mark == kind {
				return layer
			}
		}
		t.Fatalf("missing watermark kind %v", kind)
		return imaging.PreviewLayer{}
	}
	move := func(kind, other imaging.WatermarkKind, label string, dx, dy float32) {
		t.Helper()
		before, fixed := layer(kind), layer(other)
		h := control(native.RoleButton, label)
		x, y := h.Bounds.X+h.Bounds.W/2, h.Bounds.Y+h.Bounds.H/2
		window.Press(x, y)
		for i := 1; i <= 4; i++ {
			f := float32(i) / 4
			window.Move(x+f*dx, y+f*dy)
			window.Frame()
			checkBase(label, 2)
			if layer(kind).Pixels != before.Pixels || layer(other).Bounds != fixed.Bounds || m.Rendering {
				t.Fatal("position drag re-rendered a layer or moved the other mark")
			}
		}
		window.Release(x+dx, y+dy)
		window.Frame()
		after := layer(kind).Bounds.Min.Sub(before.Bounds.Min)
		if float32(after.X)*dx <= 0 || float32(after.Y)*dy <= 0 {
			t.Fatalf("%s did not follow the pointer: %v", label, after)
		}
	}
	move(imaging.TextWatermark, imaging.LogoWatermark, "拖动文字水印", -80, -48)
	move(imaging.LogoWatermark, imaging.TextWatermark, "拖动 Logo 水印", 105, 58)
	if !m.Image.Text.Free.Active || !m.Image.Logo.Free.Active || !window.HasText("自由位置") {
		t.Fatal("dragging did not update both placement modes")
	}
	clickControl(native.RoleButton, l10n.T(l10n.Default, "anchor.center"))
	if m.Image.Logo.Free.Active || m.Image.Logo.Anchor != 4 || !m.Image.Text.Free.Active {
		t.Fatal("nine-grid selection should restore only the current mark's anchor")
	}
	click("移除 Logo")
	checkBase("remove only the Logo layer", 1)
	control(native.RoleSlider, "宽度比例 (%)")
	if m.Image.Logo.Enabled || m.Image.Logo.Path != "" || testkit.Hash(t, photo) != photoHash || testkit.Hash(t, logo) != logoHash {
		t.Fatal("removing a Logo should disable only that layer and preserve both files")
	}

	m.AddLogos([]string{logo})
	wait("restore Logo", func() bool { return len(m.AfterFrame(true).Layers) == 2 })
	window.Move(500, 100)
	window.Advance(time.Second)
	click("文字")
	clickControl(native.RoleTextField, "文字内容")
	window.Key(native.Cmd, native.KeyA)
	window.Key(0, native.KeyBackspace)
	window.Frame()
	checkBase("clear only the text layer", 1)
	control(native.RoleSlider, "不透明度 (%)")
	if m.Image.Text.Enabled || m.Image.Text.Text != "" {
		t.Fatal("text was not cleared")
	}
	click("格式")
	wait("exact output with Logo only", func() bool { return !m.Rendering && m.Preview.After != nil })
	if m.Mode != app.Encoded || m.AfterFrame(false).Image != m.Preview.After {
		t.Fatal("format must automatically show the encoded result")
	}
	click("水印")
	checkBase("after the encoded proof completes", 1)
	click("Logo")
	click("移除 Logo")
	checkBase("both watermark inputs empty", 0)
	m.SetOutput(files.Settings{Directory: t.TempDir(), Conflict: files.Rename})
	click("导出全部 1 张")
	wait("export with no watermarks", func() bool { return !m.Working && m.Progress.Success == 1 })
	if testkit.Hash(t, photo) != photoHash || testkit.Hash(t, logo) != logoHash {
		t.Fatal("export modified the photo or Logo")
	}
}

func TestLiveCanvasDrawsOpacityWithoutClearingBase(t *testing.T) {
	base := testkit.Solid(100, 100, color.NRGBA{R: 20, G: 30, B: 160, A: 255})
	logo := testkit.Solid(40, 40, color.NRGBA{R: 220, G: 30, B: 30, A: 255})
	frame := app.PreviewFrame{
		Image: base, Info: imaging.Info{Width: 100, Height: 100},
		Layers: []imaging.PreviewLayer{{Pixels: logo, Bounds: image.Rect(20, 20, 60, 60), Opacity: 25}},
	}
	w := native.NewTester(func(c *native.Context) {
		theme.Apply(c)
		drawPreviewFrame(c, frame, 100, 100, "处理后")
	}, 100, 100)
	a := w.Image().RGBAAt(38, 40)
	outside := w.Image().RGBAAt(75, 80)
	frame.Layers[0].Opacity = 75
	w.Frame()
	b := w.Image().RGBAAt(38, 40)
	if b.R <= a.R+50 || b.B >= a.B-30 || w.Image().RGBAAt(75, 80) != outside || outside.B < 140 {
		t.Fatalf("expected only the overlaid pixels to change: inside %v → %v; outside %v → %v", a, b, outside, w.Image().RGBAAt(75, 80))
	}
}
