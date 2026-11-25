//go:build imageflowqa

package ui

import (
	"fmt"
	"image"
	"path/filepath"

	"imageflow/internal/app"
	"imageflow/internal/crop"
	"imageflow/internal/files"
	"imageflow/internal/imaging"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func captureEditScope(d *desktop, fixtures string, step func(string, func(), func() bool) error) error {
	m := d.model
	path := func(i int) string { return m.Sources[i].Path }
	ready := func() bool { return m.OriginalReady() && !m.Scanning && !m.Rendering && m.PreviewErr == nil }
	if err := step("01-single-crop", func() {
		opts := m.Image
		opts.Format = imaging.JPEG
		m.SetImage(opts)
		m.SetOutput(files.Settings{Directory: filepath.Join(fixtures, "export"), Conflict: files.Rename})
		d.importPaths([]string{filepath.Join(fixtures, "images")})
	}, func() bool {
		if !ready() || len(m.Sources) != 4 || m.Thumbnail(path(3)).Image == nil {
			return false
		}
		if d.page != resize {
			d.navigate(resize)
		}
		if !d.cropReady() {
			return false
		}
		d.crop.ratio = crop.FourThree
		d.crop.box = image.Rect(68, 10, 916, 646)
		d.redraw()
		return true
	}); err != nil {
		return err
	}
	if err := step("02-multiple-watermark", func() {
		d.endCrop(true)
		d.navigate(watermark)
		d.pickThumbnail(path(2), 0)
		v := m.Image
		v.Text.Size = 64
		m.EditImage(v, app.HistoryText, 0)
		d.pickThumbnail(path(1), 0)
		d.pickThumbnail(path(2), ui.Cmd)
		v = m.Image
		v.Text.Text = "技术样本"
		m.EditImage(v, app.HistoryText, 0)
	}, func() bool { return ready() && len(m.AfterFrame(true).Layers) == 1 && d.hasMixed("watermark.fontSize") }); err != nil {
		return err
	}
	if err := step("03-all-format", func() {
		d.navigate(format)
		m.SelectAll()
		v := m.Image
		v.Format = imaging.WebP
		m.EditImage(v, app.HistoryFormat, 0)
	}, func() bool { return ready() && m.Preview.After != nil && !m.Planning }); err != nil {
		return err
	}
	const title = "立式管道泵 · 出厂实拍"
	if err := step("04-single-title", func() {
		d.pickThumbnail(path(3), 0)
		d.navigate(captionPage)
		v := m.Caption()
		v.Text = title
		m.SetCaption(v)
	}, func() bool { return ready() && len(m.AfterFrame(true).Layers) == 2 }); err != nil {
		return err
	}
	if err := step("05-batch-export", func() { d.navigate(output) }, func() bool { return !m.Planning && len(m.Plan.Entries) == 4 && m.Thumbnail(path(3)).Image != nil }); err != nil {
		return err
	}
	if err := step("06-results", func() { d.start(nil) }, func() bool { return !m.Working && m.Progress.Done() == 4 }); err != nil {
		return err
	}
	var err error
	mygo.RunOnMain(func() {
		if m.Progress.Success != 4 {
			err = fmt.Errorf("four-photo export: %v", m.Report.Results)
		}
	})
	return err
}
