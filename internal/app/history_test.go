package app

import (
	"slices"
	"testing"

	"imageflow/internal/certification"
	"imageflow/internal/crop"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/series"
)

func TestHistoryRestoresMixedBatchEditsInOrder(t *testing.T) {
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	m := New(func(func()) {}, func() {})
	defer m.Close()
	m.Sources = []files.Source{
		{Path: "/fixtures/pump.png"},
		{Path: "/fixtures/book.tiff", Ordinal: 1, Series: series.Info{Kind: series.Pages, Count: 3}},
		{Path: "/fixtures/motor.png", Ordinal: 2},
	}
	m.Select(m.Sources[1].Path)
	initial := slices.Clone(m.Sources)
	oldImage, oldNames, oldOutput := m.Image, m.Naming, m.Output
	badges := certification.Builtins()
	g := certification.Defaults()
	g.Toggle(badges[0])
	m.SelectAll()
	m.SetCertification(g)
	m.Select(m.Sources[1].Path)
	solo := g
	solo.Toggle(badges[1])
	m.SetCertification(solo)
	m.ApplyCrop(map[string]bool{initial[0].Path: true, initial[1].Path: true}, crop.Region{Ratio: crop.FourThree})
	m.SetSeriesSelection(series.Selection{Scope: series.Custom, Range: "2-3", Separate: true})
	m.SetPreviewPart(2)
	image := m.Image
	image.Resize.Enabled = true
	m.EditImage(image, HistoryResize, 0)
	image.Format = imaging.PNG
	m.EditImage(image, HistoryFormat, 0)
	image.Text.Text = "R&D"
	m.EditImage(image, HistoryText, 0)
	names := m.Naming
	names.KeepOriginal, names.Prefix = false, "part-"
	m.SetNaming(names)
	output := m.Output
	output.Conflict, output.KeepTree = files.Skip, true
	m.SetOutput(output)
	m.Remove(map[string]bool{initial[1].Path: true})
	final := slices.Clone(m.Sources)
	steps := []HistoryAction{HistoryCertification, HistoryCertification, HistoryCrop, HistorySeries, HistoryResize, HistoryFormat, HistoryText, HistoryRename, HistoryOutput, HistoryRemove}
	for i := len(steps) - 1; i >= 0; i-- {
		if m.UndoAction() != steps[i] || !m.Undo() {
			t.Fatalf("wrong undo order at %s: %s", steps[i], m.UndoAction())
		}
		switch steps[i] {
		case HistoryRemove:
			if m.SelectedSource().Path != initial[1].Path || m.PreviewPart != 2 {
				t.Fatal("restored selected TIFF lost its preview page")
			}
		}
	}
	if !slices.Equal(m.Sources, initial) || m.Image != oldImage || m.Naming != oldNames || m.Output != oldOutput ||
		m.Certification().Count() != 0 || m.Undo() {
		t.Fatal("undo sequence did not recover the initial batch")
	}
	for _, step := range steps {
		if m.RedoAction() != step || !m.Redo() {
			t.Fatalf("wrong redo order at %s: %s", step, m.RedoAction())
		}
	}
	if !slices.Equal(m.Sources, final) || m.Image != oldImage || m.Naming != oldNames || m.Output != output ||
		m.Certification() != g || m.Redo() {
		t.Fatal("redo sequence did not recover the edited batch")
	}
}

func TestHistoryRetainsNewestBulkStepsAndBlocksReplayDuringJobs(t *testing.T) {
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	m := New(func(func()) {}, func() {})
	defer m.Close()
	for i := range historyLimit + 25 {
		r := m.Naming
		r.Start = i + 2
		m.SetNaming(r)
	}
	for _, busy := range []*bool{&m.Working, &m.Scanning, &m.LogoPending, &m.CertificationImporting, &m.Collage.Exporting} {
		*busy = true
		if m.Undo() || m.Redo() {
			t.Fatal("an in-flight job replayed an edit")
		}
		*busy = false
	}
	count := 0
	for m.Undo() {
		count++
	}
	if count != historyLimit || m.Naming.Start != 26 {
		t.Fatalf("old history was not dropped correctly: %d steps, start %d", count, m.Naming.Start)
	}
	r := m.Naming
	r.Prefix = "new-"
	m.SetNaming(r)
	if m.Redo() || m.RedoAction() != "" {
		t.Fatal("new edit must discard the abandoned future")
	}
}
