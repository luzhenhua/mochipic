package ui

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
)

type updateControls struct {
	canSchedule bool
	automatic   bool
	busy        bool
	installing  bool
	restart     bool
	launcher    string
	timer       *time.Timer
}

func (d *desktop) configureUpdates() {
	d.updates = &updateControls{}
	d.configurePlatformUpdates()
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !d.allowQuit() {
			e.PreventDefault()
		}
	})
}

func (d *desktop) updateButton(c *ui.Context) {
	label := "update.check"
	if d.updates != nil && d.updates.installing {
		label = "update.installing"
	}
	if button(c, tr(c, label), nil, action.Default, !d.canCheckUpdates()) {
		d.checkUpdates(true)
	}
}

func (d *desktop) automaticUpdateSetting(c *ui.Context) {
	line(c)
	setting(c, tr(c, "update.automatic"), tr(c, "update.automaticHint"), func() {
		enabled := d.updates != nil && d.updates.canSchedule
		value := enabled && d.updates.automatic
		choice.Switch(c, &value, choice.SwitchProps{}).Label(tr(c, "update.automatic")).Disabled(!enabled)
		if enabled && value != d.updates.automatic {
			d.updates.automatic = value
			d.setAutomaticUpdates(value)
		}
	})
}

func (d *desktop) allowQuit() bool {
	if !d.model.Working && !d.model.Collage.Exporting {
		return true
	}
	if !d.quitWarning {
		d.quitWarning = true
		opts := mygo.MessageOptions{
			Parent: d.win, Type: mygo.MessageWarning,
			Message: d.t("update.processing"), Detail: d.t("update.processingHint"),
		}
		go func() {
			_, _ = mygo.Dialog.Message(opts)
			mygo.RunOnMain(func() { d.quitWarning = false })
		}()
	}
	return false
}
