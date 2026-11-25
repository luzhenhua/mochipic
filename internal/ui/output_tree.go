package ui

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"

	"imageflow/internal/files"
	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type treeFolder struct {
	path    string
	dirs    []*treeFolder
	entries []*files.Entry
}
type treeRow struct {
	folder *treeFolder
	entry  *files.Entry
	depth  int
}

func (d *desktop) treeRows(entries []files.Entry) []treeRow {
	if len(entries) == 0 {
		return nil
	}
	if d.treeSource != &entries[0] {
		root := &treeFolder{}
		nodes := map[string]*treeFolder{"": root}
		var dir func(string) *treeFolder
		dir = func(path string) *treeFolder {
			if path == "." {
				path = ""
			}
			if n := nodes[path]; n != nil {
				return n
			}
			parent := dir(filepath.Dir(path))
			n := &treeFolder{path: path}
			parent.dirs = append(parent.dirs, n)
			nodes[path] = n
			return n
		}
		for i := range entries {
			entry := &entries[i]
			n := dir(filepath.Dir(entry.Relative))
			n.entries = append(n.entries, entry)
		}
		d.treeSource, d.outputTree = &entries[0], root
	}
	rows := make([]treeRow, 0, len(entries))
	var walk func(*treeFolder, int)
	walk = func(n *treeFolder, depth int) {
		rows = append(rows, treeRow{folder: n, depth: depth})
		if d.collapsedFolders[n.path] {
			return
		}
		for _, e := range n.entries {
			rows = append(rows, treeRow{entry: e, depth: depth + 1})
		}
		for _, f := range n.dirs {
			walk(f, depth+1)
		}
	}
	walk(d.outputTree, 0)
	return rows
}

func (d *desktop) drawTreeRow(c *ui.Context, r treeRow) {
	tk := theme.TokensOf(c)
	depth := float32(min(6, r.depth))
	if f := r.folder; f != nil {
		isClosed := d.collapsedFolders[f.path]
		path := f.path
		name := filepath.Base(path)
		if path == "" {
			path = d.model.Output.Directory
			name = baseName(c, path)
			if path == "" {
				name = tr(c, "output.noFolder")
			}
		}
		row := ui.ButtonBase(c).Height(47).Padding(0, 10, 0, 10+depth*24).Gap(8).
			AlignItems(ui.Center).Justify(ui.Start).Radius(tk.BorderRadius).Label(trf(c, choose(isClosed, "output.expandFolder", "output.collapseFolder"), l10n.Args{"folder": name})).Tooltip(path)
		if row.Hovered() {
			row.Background(tk.FillQuaternary)
		}
		row.Children(func() {
			icon(c, choose(isClosed, lucide.ChevronRight(), lucide.ChevronDown()), 13, tk.TextSecondary)
			icon(c, choose(isClosed, lucide.Folder(), lucide.FolderOpen()), 16, tk.TextSecondary)
			ui.Text(c, name).SingleLine().MinWidth(0).Shrink(1)
		})
		if row.Clicked() {
			if d.collapsedFolders == nil {
				d.collapsedFolders = map[string]bool{}
			}
			d.collapsedFolders[f.path] = !isClosed
		}
		return
	}
	e := r.entry
	name := filepath.Base(e.Relative)
	tip := e.Relative
	if e.Relative == "" {
		name = baseName(c, e.Source.Path)
	}
	if e.Issue != nil {
		tip = errText(c, e.Issue)
	}
	row := ui.ButtonBase(c).Height(57).Padding(0, 8, 0, 8+depth*12).Gap(8).
		AlignItems(ui.Center).Radius(tk.BorderRadius).Label(name).Tooltip(tip)
	if e.Source.Path == d.model.Selected && (e.Source.Part == 0 || e.Source.Part == d.model.PreviewPart) || row.Hovered() {
		row.Background(tk.FillSecondary)
	}
	row.Children(func() {
		d.thumb(c.Key("output-thumb"), e.Source.ID(), 46, 34)
		strong(c, name).FontSize(12).Grow(1).Basis(0).MinWidth(0).Tooltip(tip)
		desc, details, size := d.outputEdits(c, e.Source)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			muted(c, desc).MinWidth(0).Tooltip(desc)
			if details != "" {
				muted(c, details).FontSize(11).MinWidth(0).Tooltip(details)
			}
		})
		mono(c, size).FontSize(11).Shrink(0)
		entryTag(c, e.Issue, e.Skip, e.Requested != e.Relative)
	})
	if row.Clicked() {
		d.pickThumbnail(e.Source.Path, row.ClickModifiers())
		if d.model.Selected == e.Source.Path {
			d.model.SetPreviewPart(e.Source.Part)
		}
	}
}

func (d *desktop) outputEdits(c *ui.Context, s files.Source) (string, string, string) {
	// A retained naming snapshot may predate a crop, caption or layer edit.
	if s.Ordinal >= 0 && s.Ordinal < len(d.model.Sources) && d.model.Sources[s.Ordinal].Path == s.Path {
		part := s.Part
		s = d.model.Sources[s.Ordinal]
		s.Part = part
	}
	o := d.model.PhotoSettings(s).Image
	var edits, text []string
	if s.Crop.Active() {
		label := tr(c, "crop.tab")
		if u := s.Crop.Ratio.Units(); u.X > 0 {
			label += fmt.Sprintf(" %d:%d", u.X, u.Y)
		}
		edits = append(edits, label)
	}
	if o.Resize.Enabled {
		edits = append(edits, tr(c, "crop.resizeTab"))
	}
	if o.Text.Enabled {
		edits = append(edits, tr(c, "watermark.text"))
		text = append(text, o.Text.Text)
	}
	if len(o.LogoMarks()) > 0 {
		edits = append(edits, tr(c, "watermark.logo"))
	}
	if s.Certification.Count() > 0 {
		edits = append(edits, tr(c, "nav.certification"))
	}
	if s.Caption.Active() {
		edits = append(edits, tr(c, "caption.strip"))
		text = append(text, s.Caption.Text)
	}
	if len(edits) == 0 {
		edits = append(edits, tr(c, "resize.originalSize"))
	}
	size := "—"
	info := d.model.Thumbnail(s.ID()).Info
	if s.Series.Kind == series.Pages && s.Series.Multiple() && s.Part == 0 {
		size = tr(c, "scope.byPage")
	} else if info.Width > 0 {
		b := s.Crop.Resolve(image.Pt(info.Width, info.Height))
		if g, err := o.Resize.Geometry(b.Dx(), b.Dy()); err == nil {
			size = sizeText(g.Width, g.Height)
		}
	}
	return strings.Join(edits, " · "), strings.Join(text, " · "), size
}
