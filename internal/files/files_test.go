package files

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"imageflow/internal/l10n"
	"imageflow/internal/series"
	"imageflow/internal/testkit"
)

func TestRules(t *testing.T) {
	r := Rules{Prefix: "CY_", Suffix: "-A", Template: "{date}-{prefix}{original}{suffix}_{index}", Sequence: true, Start: 7, Digits: 4}
	got, err := r.Name("目录/泵.JPG", ".webp", 2, time.Date(2026, 11, 10, 0, 0, 0, 0, time.Local))
	if err != nil || got != "20261110-CY_泵-A_0009.webp" {
		t.Fatalf("%q %v", got, err)
	}
	r.KeepOriginal = true
	if got, e := r.Name("泵.JPG", ".jpg", 0, time.Now()); got != "泵.jpg" || e != nil {
		t.Fatalf("%q %v", got, e)
	}
	for _, s := range []string{"../escape", "/tmp/escape", "..", "{unknown}", "{prefix", "}", "\x00", "a:b", "a\\b"} {
		r.KeepOriginal, r.Template = false, s
		if n, e := r.Name("a.png", ".png", 0, time.Now()); e == nil {
			t.Errorf("accepted %q as %q", s, n)
		}
	}
}

func TestPlanAndExclusiveCommit(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "exports")
	if e := os.Mkdir(out, 0o700); e != nil {
		t.Fatal(e)
	}
	orig := filepath.Join(tmp, "Pump.png")
	if e := os.WriteFile(orig, []byte("original"), 0o600); e != nil {
		t.Fatal(e)
	}
	before := testkit.Hash(t, orig)
	existing := filepath.Join(out, "PUMP.webp")
	if e := os.WriteFile(existing, []byte("existing"), 0o600); e != nil {
		t.Fatal(e)
	}
	existingHash := testkit.Hash(t, existing)
	out, _ = filepath.EvalSymlinks(out)
	src := []Source{{Path: orig}, {Path: filepath.Join(tmp, "b", "pump.jpg"), Ordinal: 1}}
	p, e := BuildPlan(t.Context(), src, Settings{Directory: out, Conflict: Rename}, DefaultRules(), ".webp", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if p.Entries[0].Relative != "Pump (1).webp" || p.Entries[1].Relative != "pump (2).webp" {
		t.Fatalf("%+v", p.Entries)
	}
	d, e := OpenDestination(p)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	late := filepath.Join(out, "Pump (1).webp")
	if e = os.WriteFile(late, []byte("created after planning"), 0o600); e != nil {
		t.Fatal(e)
	}
	lateHash := testkit.Hash(t, late)
	path, e := d.Commit(t.Context(), p.Entries[0], func(w io.Writer) error { _, e := io.WriteString(w, "new"); return e })
	if e != nil || filepath.Base(path) != "Pump (3).webp" {
		t.Fatalf("%s %v", path, e)
	}
	if testkit.Hash(t, orig) != before || testkit.Hash(t, existing) != existingHash || testkit.Hash(t, late) != lateHash {
		t.Fatal("original or existing destination changed")
	}
	if p, e := BuildPlan(t.Context(), src, Settings{Directory: out, Conflict: Skip}, DefaultRules(), ".webp", time.Now()); e != nil || !p.Entries[0].Skip {
		t.Fatalf("skip plan %+v %v", p, e)
	}
	if _, e := BuildPlan(t.Context(), src, Settings{Directory: tmp, Conflict: Rename}, DefaultRules(), ".webp", time.Now()); e == nil {
		t.Fatal("output can contain input")
	}
	assertNoTemp(t, out)
}

func TestRefuseSymlinkTreeAndCleanup(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out")
	private := filepath.Join(tmp, "unrelated")
	for _, p := range []string{out, private} {
		if e := os.Mkdir(p, 0o700); e != nil {
			t.Fatal(e)
		}
	}
	testkit.DirectoryLink(t, private, filepath.Join(out, "nested"))
	src := []Source{{Path: filepath.Join(tmp, "a.jpg"), RelativeDir: "nested"}, {Path: filepath.Join(tmp, "b.jpg"), RelativeDir: "../escape"}, {Path: filepath.Join(tmp, "c.jpg")}}
	p, e := BuildPlan(t.Context(), src, Settings{Directory: out, Conflict: Rename, KeepTree: true}, DefaultRules(), ".png", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if p.Entries[0].Issue == nil || p.Entries[1].Issue == nil || p.Entries[2].Issue != nil {
		t.Fatalf("unsafe plan: %+v", p.Entries)
	}
	d, e := OpenDestination(p)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	injected := errors.New("encode failed")
	if _, e = d.Commit(t.Context(), p.Entries[2], func(w io.Writer) error { _, _ = io.WriteString(w, "partial"); return injected }); !errors.Is(e, injected) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if _, e = d.Commit(ctx, p.Entries[2], func(w io.Writer) error { _, _ = io.WriteString(w, "partial"); cancel(); return nil }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(out, p.Entries[2].Relative)); !os.IsNotExist(e) {
		t.Fatal("partial output published")
	}
	assertNoTemp(t, out)
}

func TestCollagePlanProtectsEvenUnselectedOriginals(t *testing.T) {
	out := t.TempDir()
	unselected := Source{
		Path:   filepath.Join(out, "existing.tiff"),
		Series: series.Info{Kind: series.Pages, Count: 2},
	}
	if err := os.WriteFile(unselected.Path, []byte("original TIFF sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	originals := []Source{{Path: filepath.Join(t.TempDir(), "material.png")}, unselected}
	before := testkit.Hash(t, unselected.Path)
	settings := Settings{Directory: out, Conflict: Rename, KeepTree: true}
	if _, err := BuildCollagePlan(t.Context(), originals, settings, ".png", time.Now()); err == nil ||
		l10n.ErrorText(l10n.Default, err) != l10n.T(l10n.Default, "error.outputContainsSource") {
		t.Fatalf("did not reject a destination containing an unselected original: %v", err)
	}
	settings.Directory = t.TempDir()
	plan, err := BuildCollagePlan(t.Context(), originals, settings, ".png", time.Now())
	if err != nil || len(plan.Entries) != 1 || plan.Entries[0].Relative != "collage.png" {
		t.Fatalf("an unrelated TIFF without a page selection blocked the collage: %+v, %v", plan.Entries, err)
	}
	if testkit.Hash(t, unselected.Path) != before {
		t.Fatal("unselected original changed")
	}
}

func TestRejectUnwritableDestination(t *testing.T) {
	out := t.TempDir()
	src := filepath.Join(t.TempDir(), "source.jpg")
	p, e := BuildPlan(t.Context(), []Source{{Path: src}}, Settings{Directory: out, Conflict: Rename}, DefaultRules(), ".png", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	denyCreatingFiles(t, out)
	d, e := OpenDestination(p)
	if e == nil {
		d.Close()
		t.Fatal("opened unwritable directory")
	}
	assertNoTemp(t, out)
}
func assertNoTemp(t *testing.T, path string) {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(path, ".imageflow-*"))
	if len(m) > 0 {
		t.Fatalf("temporary files left behind: %v", m)
	}
}
