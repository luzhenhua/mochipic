// Package config persists versioned, local user preferences.
package config

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"imageflow/internal/batch"
	"imageflow/internal/caption"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
)

const Version = 1

type Preset struct {
	ID, Name string
	Image    imaging.Options
	Naming   files.Rules
}

const (
	factoryWeb    = "网站产品图"
	factoryDetail = "产品详情图"
	factorySocial = "社交媒体图片"
)

// DefaultNameKey returns the localized label for an unrenamed factory preset.
func (p Preset) DefaultNameKey() string {
	switch p.ID {
	case "builtin-web":
		if p.Name == factoryWeb {
			return "preset.factoryWeb"
		}
	case "builtin-detail":
		if p.Name == factoryDetail {
			return "preset.factoryDetail"
		}
	case "builtin-social":
		if p.Name == factorySocial {
			return "preset.factorySocial"
		}
	}
	return ""
}

func (p Preset) HasDefaultName() bool { return p.DefaultNameKey() != "" }

// WindowSize is the last user-chosen size of the normal window, in points.
type WindowSize struct{ Width, Height int }

type Preferences struct {
	Version          int
	Theme            string
	Language         string
	Format           imaging.Format
	Output           files.Settings
	Workers          int
	Recursive        bool
	AutomaticUpdates bool `json:",omitzero"`
	DefaultPreset    string
	Presets          []Preset
	WindowSize       WindowSize    `json:",omitzero"`
	CaptionStyle     caption.Style `json:",omitzero"`
}

func Defaults() Preferences {
	web := imaging.Defaults()
	web.Resize.Enabled = true
	detail := web
	detail.Resize.Mode = imaging.LongEdge
	detail.Format, detail.Quality = imaging.JPEG, 92
	social := web
	social.Resize.Width, social.Resize.Height, social.Resize.Fit = 1080, 1080, imaging.Cover
	social.Format, social.Quality = imaging.JPEG, 90
	r := files.DefaultRules()
	return Preferences{Version: Version, Theme: "system", Language: l10n.Default, Format: imaging.WebP, Output: files.Settings{Conflict: files.Rename}, Workers: 2, Recursive: true,
		Presets: []Preset{{"builtin-web", factoryWeb, web, r}, {"builtin-detail", factoryDetail, detail, r}, {"builtin-social", factorySocial, social, r}}}
}

func (p Preferences) Clone() Preferences { p.Presets = slices.Clone(p.Presets); return p }

func (p Preferences) Preset(id string) (Preset, bool) {
	for _, preset := range p.Presets {
		if preset.ID == id {
			return preset, true
		}
	}
	return Preset{}, false
}

func (p Preferences) Validate() error {
	if p.Version != Version {
		return l10n.Error("error.preferencesVersion", l10n.Args{"version": p.Version})
	}
	if p.CaptionStyle != (caption.Style{}) && !p.CaptionStyle.Valid() {
		return l10n.New("error.captionSettings")
	}
	if (p.Theme != "system" && p.Theme != "light" && p.Theme != "dark") ||
		!l10n.Supported(p.Language) || p.Format.Extension() == "" || p.Workers < 1 || p.Workers > batch.MaxWorkers ||
		p.Output.Conflict != files.Rename && p.Output.Conflict != files.Skip ||
		p.Output.Directory != "" && !filepath.IsAbs(p.Output.Directory) || len(p.Presets) > 64 {
		return l10n.New("error.invalidPreferences")
	}
	ids := map[string]bool{}
	for _, pr := range p.Presets {
		if pr.ID == "" || ids[pr.ID] || strings.TrimSpace(pr.Name) == "" || len(pr.Name) > 180 {
			return l10n.New("error.invalidPreset")
		}
		// Assets may move after saving a preset. Their availability is verified
		// only when the preset is applied, not when opening preferences.
		if err := pr.Image.Validate(); err != nil {
			return l10n.WrapWithArgs("error.presetDetails", err, l10n.Args{"name": pr.Name})
		}
		if _, err := pr.Naming.Name("sample.jpg", pr.Image.Format.Extension(), 0, time.Now()); err != nil {
			return l10n.WrapWithArgs("error.presetDetails", err, l10n.Args{"name": pr.Name})
		}
		ids[pr.ID] = true
	}
	if p.DefaultPreset != "" && !ids[p.DefaultPreset] {
		return l10n.New("error.defaultPresetMissing")
	}
	return nil
}

func Path() (string, error) {
	dir := os.Getenv("IMAGEFLOW_CONFIG_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "ImageFlow")
	}
	return filepath.Join(dir, "preferences.json"), nil
}

func Load(path string) (Preferences, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), err
	}
	defer f.Close()
	const limit = 1 << 20
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return Defaults(), err
	}
	if len(b) > limit {
		return Defaults(), l10n.New("error.preferencesTooLarge")
	}
	p := Defaults()
	if err := json.Unmarshal(b, &p); err != nil {
		return Defaults(), err
	}
	if !l10n.Supported(p.Language) {
		p.Language = l10n.Default
	}
	if err := p.Validate(); err != nil {
		return Defaults(), err
	}
	return p, nil
}

func Save(path string, p Preferences) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".preferences-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(append(b, '\n')); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
