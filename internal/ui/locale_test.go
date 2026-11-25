package ui

import (
	"testing"
	"time"

	"imageflow/internal/app"
	"imageflow/internal/config"
	"imageflow/internal/l10n"

	native "github.com/egoist/mygo/ui"
)

func TestLanguageSelectorSavesChoice(t *testing.T) {
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	func() {
		updates := make(chan func(), 8)
		d := &desktop{page: settings}
		d.model = app.New(func(fn func()) { updates <- fn }, d.redraw)
		defer func() {
			if err := d.model.Close(); err != nil {
				t.Error(err)
			}
		}()
		w := native.NewTester(d.view, 980, 630)
		w.SetLocale(l10n.Default)
		if !w.HasText("麻薯图图") || !w.HasText(Version) {
			t.Fatalf("Chinese name or version missing: %v", w.Texts())
		}
		if err := w.Click("简体中文"); err != nil {
			t.Fatal(err)
		}
		w.Advance(250 * time.Millisecond)
		if err := w.Click("日本語"); err != nil {
			t.Fatal(err)
		}
		if got := d.model.Preferences.Language; got != "ja" {
			t.Fatalf("language choice not applied: %q", got)
		}
		// A headless Tester owns its locale separately from MyGo's running App.
		w.SetLocale("ja")
		if !w.HasText("設定") || !w.HasText("言語") || !w.HasText("日本語") || !w.HasText("MochiPic") || w.HasText("麻薯图图") || w.HasText("默认输出") {
			t.Fatalf("settings did not redraw in Japanese: %v", w.Texts())
		}
		if err := w.Click("読み込みに戻る"); err != nil {
			t.Fatal(err)
		}
		if !w.HasText("画像を選択…") || !w.HasText("フォルダを選択…") || w.HasText("选择图片…") {
			t.Fatalf("import page did not use the selected locale: %v", w.Texts())
		}
	}()
	got, err := config.Load(path)
	if err != nil || got.Language != "ja" {
		t.Fatalf("language choice was not saved: %q, %v", got.Language, err)
	}
}

func TestLocalizedFactoryPresetNamesKeepCustomNames(t *testing.T) {
	preset := config.Defaults().Presets[0]
	if name := localizedPresetName("en", preset); name != "Web product photos" {
		t.Fatalf("factory preset was not localized: %q", name)
	}
	preset.Name = "产品组 A"
	if name := localizedPresetName("en", preset); name != preset.Name {
		t.Fatalf("user's preset name was changed: %q", name)
	}
}
