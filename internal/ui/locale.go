package ui

import (
	"runtime"

	"imageflow/internal/config"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/ja"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
)

func init() { l10n.Register() }

func tr(c *ui.Context, key string) string { return l10n.T(c.Locale(), key) }

func trf(c *ui.Context, key string, args l10n.Args) string { return l10n.Format(c.Locale(), key, args) }

func errText(c *ui.Context, err error) string { return l10n.ErrorText(c.Locale(), err) }

func (d *desktop) locale() string {
	if d.win == nil {
		return d.model.Preferences.Language
	}
	return mygo.App.Locale()
}

func (d *desktop) t(key string) string { return l10n.T(d.locale(), key) }

func presetDisplayName(c *ui.Context, p config.Preset) string {
	return localizedPresetName(c.Locale(), p)
}

func localizedPresetName(lang string, p config.Preset) string {
	if key := p.DefaultNameKey(); key != "" {
		return l10n.T(lang, key)
	}
	return p.Name
}

func shortcutModifier() string {
	if runtime.GOOS == "darwin" {
		return "⌘"
	}
	return "Ctrl"
}

func shortcutLabel(key string) string {
	if runtime.GOOS == "darwin" {
		return "⌘" + key
	}
	return shortcutModifier() + "+" + key
}

func trafficLights() *mygo.Point {
	if runtime.GOOS == "darwin" {
		return &mygo.Point{X: 12, Y: 12}
	}
	return nil
}
