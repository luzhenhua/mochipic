package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/files"
	"imageflow/internal/l10n"
	"imageflow/internal/testkit"
)

func TestNativeImportPreviouslyExportedFolderWithDefaultOutputSet(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	parent, freshOutput := t.TempDir(), t.TempDir()
	chuli := filepath.Join(parent, "chuli")
	if err := os.Mkdir(chuli, 0o700); err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(chuli, "01.jpg"), filepath.Join(chuli, "02.jpg")
	testkit.JPEG(t, first, testkit.Solid(100, 80, color.NRGBA{G: 170, A: 255}))
	testkit.JPEG(t, second, testkit.Solid(96, 72, color.NRGBA{B: 160, A: 255}))
	before := testkit.Hash(t, first)
	other := testkit.Hash(t, second)
	prefs := m.Preferences.Clone()
	prefs.Output = files.Settings{Directory: chuli, Conflict: files.Rename}
	m.SetPreferences(prefs)
	d.newBatch()
	w.Frame()

	drop := func(p string) {
		t.Helper()
		if !w.DropFiles(560, 360, p) {
			t.Fatalf("view did not accept %s", p)
		}
		w.Frame()
	}
	drop(parent)
	wait("parent scan completed", func() bool { return !m.Scanning })
	hint := l10n.T(l10n.Default, "import.outputFolderSkipped")
	if d.page != importHome || len(m.Sources) != 0 || !w.HasText(hint) {
		t.Fatalf("parent scan should skip output and explain how to import it: %+v %v", m.Scan, w.Texts())
	}

	drop(chuli)
	wait("chosen folder imported", func() bool { return !m.Scanning && !m.Planning && m.OriginalReady() })
	if d.page != pictures || len(m.Sources) != 2 || m.Scan.Errors != 0 {
		t.Fatalf("explicit output folder was not imported: %+v", m.Scan)
	}
	if l10n.ErrorText(l10n.Default, m.PlanErr) != l10n.T(l10n.Default, "error.outputContainsSource") {
		t.Fatalf("export must still refuse the folder that now holds the sources: %v", m.PlanErr)
	}
	m.SetOutput(files.Settings{Directory: freshOutput, Conflict: files.Rename})
	wait("independent output planned", func() bool { return !m.Planning && m.PlanErr == nil && len(m.Plan.Entries) == 2 })
	clickWorkspace(t, w, "导出全部 2 张")
	wait("second processing pass finished", func() bool { return !m.Working && m.Progress.Done() == 2 })
	if m.Progress.Success != 2 || testkit.Hash(t, first) != before || testkit.Hash(t, second) != other {
		t.Fatalf("reprocessing changed the inputs or did not write both outputs: %+v", m.Progress)
	}
}
