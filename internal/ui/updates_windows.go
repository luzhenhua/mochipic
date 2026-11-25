package ui

import (
	"context"
	"log"
	"os/exec"
	"time"

	"imageflow/internal/l10n"
	"imageflow/internal/winupdate"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"
)

var allowRestartForeground = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

func (d *desktop) configurePlatformUpdates() {
	launcher, err := winupdate.RunningLauncher()
	if err != nil || !mygo.Updater.Enabled() {
		return
	}
	d.updates.launcher = launcher
	d.updates.canSchedule = true
	d.updates.automatic = d.model.Preferences.AutomaticUpdates
	mygo.App.WhenReady(func() { d.scheduleWindowsUpdate(10 * time.Second) })
	mygo.App.OnQuit(func() {
		if d.updates.timer != nil {
			d.updates.timer.Stop()
		}
	})
}

func (d *desktop) setAutomaticUpdates(enabled bool) {
	p := d.model.Preferences
	p.AutomaticUpdates = enabled
	d.model.SetPreferences(p)
	d.scheduleWindowsUpdate(10 * time.Second)
}

func (d *desktop) scheduleWindowsUpdate(delay time.Duration) {
	if d.updates.timer != nil {
		d.updates.timer.Stop()
	}
	if d.updates.automatic {
		d.updates.timer = time.AfterFunc(delay, func() {
			mygo.RunOnMain(func() {
				if d.updates.automatic {
					d.checkUpdates(false)
					d.scheduleWindowsUpdate(24 * time.Hour)
				}
			})
		})
	}
}

func (d *desktop) canCheckUpdates() bool {
	return d.updates != nil && d.updates.launcher != "" && !d.updates.busy && !d.updates.restart
}

func (d *desktop) updateMenuItem() *mygo.MenuItem {
	return &mygo.MenuItem{
		Label: d.t("update.check"), Disabled: !d.canCheckUpdates(),
		Click: func(*mygo.MenuItem, *mygo.Window) { d.checkUpdates(true) },
	}
}

func (d *desktop) refreshUpdateControls() {
	if d.win != nil {
		d.installMenu()
		d.redraw()
	}
}

func (d *desktop) checkUpdates(manual bool) {
	if !d.canCheckUpdates() {
		return
	}
	d.updates.busy = true
	d.refreshUpdateControls()
	launcher, parent, locale := d.updates.launcher, d.win, d.model.Preferences.Language
	go func() {
		installed := false
		defer func() {
			mygo.RunOnMain(func() {
				d.updates.busy, d.updates.installing = false, false
				if installed {
					// If processing started while the update downloaded, the
					// regular quit guard keeps the current workspace open.
					d.updates.launcher = ""
					d.updates.canSchedule = false
					if !d.model.Working && !d.model.Collage.Exporting {
						d.updates.restart = true
						allowRestartForeground.Call(uintptr(^uint32(0)))
						mygo.App.Quit()
					} else {
						opts := mygo.MessageOptions{Parent: parent, Type: mygo.MessageInfo,
							Message: l10n.T(locale, "update.installedNextLaunch")}
						go func() { _, _ = mygo.Dialog.Message(opts) }()
					}
				}
				d.refreshUpdateControls()
			})
		}()
		text := func(key string) string { return l10n.T(locale, key) }
		message := func(kind mygo.MessageType, text, detail string, buttons ...string) (mygo.MessageResult, error) {
			return mygo.Dialog.Message(mygo.MessageOptions{
				Parent: parent, Type: kind, Title: l10n.T(locale, "update.check"),
				Message: text, Detail: detail, Buttons: buttons,
			})
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		update, err := mygo.Updater.Check(ctx)
		cancel()
		if err != nil {
			if manual {
				_, _ = message(mygo.MessageWarning, text("update.checkError"), "")
			}
			return
		}
		if update == nil {
			if manual {
				_, _ = message(mygo.MessageInfo, text("update.current"), "")
			}
			return
		}
		found := l10n.Format(locale, "update.available", l10n.Args{"version": update.Version})
		notes := []rune(update.Notes)
		if len(notes) > 2000 {
			notes = append(notes[:2000], '…')
		}
		answer, err := message(mygo.MessageInfo, found, string(notes), text("common.cancel"), text("update.installRestart"))
		if err != nil || answer.Button != 1 {
			return
		}
		mygo.RunOnMain(func() {
			d.updates.installing = true
			d.refreshUpdateControls()
		})
		cmd := exec.Command(launcher, winupdate.ApplyCommand, update.Version)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("install Windows update: %v: %s", err, output)
			_, _ = message(mygo.MessageWarning, text("update.installError"), "")
			return
		}
		installed = true
	}()
}
