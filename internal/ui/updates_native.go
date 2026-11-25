//go:build !windows

package ui

import (
	"log"
	"os"
	"path/filepath"

	"imageflow/internal/config"
	"imageflow/internal/l10n"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	updaternative "github.com/egoist/mygo/plugins/updater/native"
)

func (d *desktop) configurePlatformUpdates() {
	strings := make(map[string]updater.Strings)
	for _, lang := range l10n.Languages() {
		strings[lang.Tag] = updater.Strings{
			MenuItem:   l10n.T(lang.Tag, "update.check"),
			CheckError: l10n.T(lang.Tag, "update.checkError"),
		}
	}
	mygo.Use(updaternative.New(updater.Options{
		Icon: brandIconPNG, DisableAutomaticChecks: true, Strings: strings,
	}))
	if mygo.Updater.Enabled() {
		path, err := config.Path()
		if err == nil {
			err = os.MkdirAll(filepath.Dir(path), 0o700)
		}
		if err != nil {
			log.Printf("updater preferences: %v", err)
		} else {
			mygo.App.SetPath(mygo.PathUserData, filepath.Dir(path))
			d.updates.canSchedule = true
			d.updates.automatic = updater.AutomaticChecks()
		}
	}
}

func (d *desktop) canCheckUpdates() bool            { return d.updates != nil }
func (d *desktop) checkUpdates(bool)                { updater.CheckForUpdates() }
func (d *desktop) updateMenuItem() *mygo.MenuItem   { return updater.MenuItem() }
func (d *desktop) setAutomaticUpdates(enabled bool) { updater.SetAutomaticChecks(enabled) }
