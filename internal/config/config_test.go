package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/imaging"
)

func TestLogoStackPresetPreservesOrderAndReadsLegacySettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.json")
	p := Defaults()
	o := &p.Presets[0].Image
	a := imaging.DefaultLogo()
	a.Enabled, a.Path = true, filepath.Join(dir, "brand.png")
	o.Logo, o.LogoOnTop = a, false
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile(path)
	if err != nil || bytes.Contains(legacy, []byte("AdditionalLogos")) {
		t.Fatalf("single Logo preset changed its persisted shape: %v", err)
	}
	read, err := Load(path)
	if err != nil || read.Presets[0].Image != *o || read.Presets[0].Image.TextLayerPosition(1) != 1 {
		t.Fatalf("legacy order or settings lost: %v", err)
	}
	b := a
	b.Path, b.Anchor, b.Opacity = filepath.Join(dir, "dealer.png"), 8, 64
	o.SetLogoMarks([]imaging.LogoMark{a, b})
	o.SetTextLayerPosition(1, 2)
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	read, err = Load(path)
	if err != nil || read.Presets[0].Image != *o || read.Presets[0].Image.TextLayerPosition(2) != 1 {
		t.Fatalf("multiple Logo settings or text order lost: %v", err)
	}
}

func TestSaveReloadAndRejectNewerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "preferences.json")
	p, err := Load(path)
	if err != nil || p.Version != Version {
		t.Fatalf("%+v %v", p, err)
	}
	p.Theme, p.Format, p.DefaultPreset = "dark", imaging.PNG, p.Presets[0].ID
	p.WindowSize = WindowSize{Width: 1060, Height: 680}
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Theme != "dark" || got.Format != imaging.PNG || got.DefaultPreset != p.DefaultPreset || got.WindowSize != p.WindowSize {
		t.Fatalf("%+v %v", got, err)
	}
	if err := os.WriteFile(path, []byte(`{"Version":44}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path); err == nil {
		t.Fatal("unknown schema accepted")
	}
}

func TestLanguagePreferenceKeepsExistingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	for _, fixture := range []string{
		`{"Version":1,"Theme":"dark","Workers":3}`,
		`{"Version":1,"Theme":"dark","Workers":3,"Language":"xx"}`,
	} {
		if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
			t.Fatal(err)
		}
		p, err := Load(path)
		if err != nil || p.Language != "zh-Hans" || p.Theme != "dark" || p.Workers != 3 {
			t.Fatalf("language migration changed preferences: %+v, %v", p, err)
		}
		p.Language = "pt-BR"
		if err := Save(path, p); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got.Language != "pt-BR" || got.Theme != p.Theme || got.Workers != p.Workers {
			t.Fatalf("language round trip: %+v, %v", got, err)
		}
	}
}
