package app

import (
	"testing"

	"imageflow/internal/config"
	"imageflow/internal/imaging"
)

func TestPresetWithOldOffSwitchDoesNotActivateHiddenWatermarks(t *testing.T) {
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	m := New(func(func()) {}, func() {})
	defer m.Close()
	o := imaging.Defaults()
	o.Text.Text, o.Logo.Path = "saved but disabled", "/test/old-logo.png"
	p := config.Preset{ID: "legacy", Image: o}
	m.ApplyPreset(p)
	if m.Image.Text.Enabled || m.Image.Logo.Enabled || m.Image.Text.Text != "" || m.Image.Logo.Path != "" {
		t.Fatal("applying an old preset reactivated disabled content")
	}
	if m.Image.Text.Placement != p.Image.Text.Placement || m.AppliedPreset != p.ID {
		t.Fatal("the preset lost its styling or selection")
	}
	live := m.Image
	live.Text.Text = "new text"
	m.SetImage(live)
	if !m.Image.Text.Enabled || m.Image.Logo.Enabled {
		t.Fatal("entering text must activate only its own layer")
	}
}
