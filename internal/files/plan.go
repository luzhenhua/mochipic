package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"imageflow/internal/caption"
	"imageflow/internal/certification"
	"imageflow/internal/crop"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/series"
)

type Conflict string

const (
	Rename Conflict = "rename"
	Skip   Conflict = "skip"
)

// PhotoSettings is immutable once attached to a source. Plans, retries and undo
// records can share it safely while the editor replaces it on the next edit.
type PhotoSettings struct {
	Image  imaging.Options
	Naming Rules
	Preset string
}

type Source struct {
	Settings          *PhotoSettings
	Path, RelativeDir string
	Size              int64
	Ordinal           int
	Series            series.Info
	Selection         series.Selection
	Part              int // Nonzero only for an individual output of a series.
	Crop              crop.Region
	Certification     certification.Group
	Caption           caption.Caption
}

func (s Source) ID() string {
	if s.Part == 0 {
		return s.Path
	}
	return s.Path + "\x00" + strconv.Itoa(s.Part)
}

func (s Source) outputExtension(still string) string {
	if s.Series.Multiple() && !s.Selection.Separate {
		if s.Series.Kind == series.Pages {
			return ".tiff"
		}
		return ".webp"
	}
	return still
}

// PrepareSources snapshots one work item per output. A retry can already
// represent an individual part and must not expand to the whole selection.
func PrepareSources(ctx context.Context, src []Source) ([]Source, error) {
	out := make([]Source, 0, min(len(src), series.MaxOutputs))
	var last Source
	var lastParts []int
	for _, s := range src {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if last.Path != s.Path || last.Series != s.Series || last.Selection != s.Selection {
			var err error
			lastParts = nil
			if s.Selection.Separate {
				lastParts, err = s.Selection.Parts(s.Series)
			} else {
				err = s.Selection.Validate(s.Series)
			}
			if err != nil {
				return nil, l10n.WrapWithArgs("error.seriesForFile", err, l10n.Args{"file": filepath.Base(s.Path)})
			}
			last = s
		}
		parts := lastParts
		if s.Part != 0 {
			if _, found := slices.BinarySearch(parts, s.Part); !s.Selection.Separate || !found {
				return nil, l10n.New("error.seriesChanged")
			}
			parts = nil
		}
		n := 1
		if s.Selection.Separate && len(parts) > 0 {
			n = len(parts)
		}
		if len(out)+n > series.MaxOutputs {
			return nil, l10n.Error("error.seriesOutputLimit", l10n.Args{"count": series.MaxOutputs})
		}
		if s.Selection.Separate && len(parts) > 0 {
			for _, part := range parts {
				s.Part = part
				out = append(out, s)
			}
		} else {
			out = append(out, s)
		}
	}
	return out, nil
}

type Settings struct {
	Directory string
	Conflict  Conflict
	KeepTree  bool
}

type Entry struct {
	Source    Source
	Relative  string
	Requested string
	Issue     error
	Skip      bool
}

type Plan struct {
	Directory string
	Conflict  Conflict
	Entries   []Entry
	rootInfo  os.FileInfo
}

type folderPlan struct {
	path     string
	names    map[string]string
	reserved map[string]bool
	next     map[string]int
	err      error
}

func snapshotFolder(d *os.File) (map[string]string, error) {
	names, err := readFolderNames(d)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(names))
	for _, n := range names {
		k := fold(n)
		if old, ok := m[k]; !ok || n < old {
			m[k] = n
		}
	}
	return m, nil
}

// BuildPlan is read-only. It reserves case-folded names in input order and
// includes existing files (including dangling links) when suggesting names.
func BuildPlan(ctx context.Context, src []Source, settings Settings, rules Rules, extension string, now time.Time) (Plan, error) {
	entries, err := PrepareSources(ctx, src)
	if err != nil {
		return Plan{Conflict: settings.Conflict}, err
	}
	return buildPlan(ctx, src, entries, settings, rules, extension, now)
}

// BuildCollagePlan reserves one output while protecting every imported
// original, including files not used in this particular collage.
func BuildCollagePlan(ctx context.Context, originals []Source, settings Settings, extension string, now time.Time) (Plan, error) {
	if len(originals) == 0 {
		return Plan{}, l10n.New("error.noImagesToProcess")
	}
	settings.KeepTree = false
	return buildPlan(ctx, originals, []Source{{Path: originals[0].Path}}, settings, Rules{Template: "collage"}, extension, now)
}

func buildPlan(ctx context.Context, originals, src []Source, settings Settings, rules Rules, extension string, now time.Time) (Plan, error) {
	p := Plan{Conflict: settings.Conflict}
	if settings.Conflict != Rename && settings.Conflict != Skip {
		return p, l10n.New("error.invalidConflictPolicy")
	}
	p.Entries = make([]Entry, 0, len(src))
	var root *os.File
	if settings.Directory != "" {
		dir, err := filepath.EvalSymlinks(settings.Directory)
		if err != nil {
			return p, l10n.Wrap("error.openOutputFolder", err)
		}
		if !filepath.IsAbs(dir) {
			return p, l10n.New("error.absoluteOutputPath")
		}
		p.Directory = dir
		parents := map[string]string{}
		for _, s := range originals {
			if ctx.Err() != nil {
				return p, ctx.Err()
			}
			parent := filepath.Dir(s.Path)
			canonical, ok := parents[parent]
			if !ok {
				canonical, _ = filepath.EvalSymlinks(parent)
				parents[parent] = canonical
			}
			if Within(s.Path, dir) || Within(s.Path, settings.Directory) ||
				canonical != "" && Within(filepath.Join(canonical, filepath.Base(s.Path)), dir) {
				return p, l10n.New("error.outputContainsSource")
			}
		}
		root, err = openAbsolute(dir)
		if err != nil {
			return p, l10n.Wrap("error.openOutputFolder", err)
		}
		defer root.Close()
		p.rootInfo, err = root.Stat()
		if err != nil {
			return p, err
		}
	}
	folders := map[string]*folderPlan{}
	var getFolder func(string) *folderPlan
	getFolder = func(path string) *folderPlan {
		if path == "." {
			path = ""
		}
		key := fold(path)
		if f := folders[key]; f != nil {
			return f
		}
		f := &folderPlan{path: path, reserved: map[string]bool{}, next: map[string]int{}}
		folders[key] = f
		if path != "" {
			parent := getFolder(filepath.Dir(path))
			if parent.err != nil {
				f.err = parent.err
				return f
			}
			name := filepath.Base(path)
			if existing, ok := parent.names[fold(name)]; ok {
				name = existing
			}
			f.path = filepath.Join(parent.path, name)
		}
		if root == nil {
			return f
		}
		dir, err := openTree(root, f.path, false)
		if errors.Is(err, os.ErrNotExist) && f.path != "" {
			return f
		}
		if err != nil {
			f.err = l10n.Wrap("error.outputSubfolderUnavailable", err)
			return f
		}
		defer dir.Close()
		f.names, err = snapshotFolder(dir)
		if err == nil && f.path != "" {
			err = checkFolderAccess(dir)
		}
		if err != nil {
			f.err = l10n.Wrap("error.outputFolderAccess", err)
		}
		return f
	}
	for _, s := range src {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		e := Entry{Source: s}
		photoRules, photoExt := rules, extension
		if s.Settings != nil {
			photoRules, photoExt = s.Settings.Naming, s.Settings.Image.Format.Extension()
		}
		ext := s.outputExtension(photoExt)
		name, err := photoRules.Name(s.Path, ext, s.Ordinal, now)
		if err == nil && s.Part != 0 {
			kind := "frame"
			if s.Series.Kind == series.Pages {
				kind = "page"
			}
			name = fmt.Sprintf("%s-%s-%04d%s", strings.TrimSuffix(name, ext), kind, s.Part, ext)
			if len(name) > 240 {
				err = l10n.New("error.invalidOutputName")
			}
		}
		rel := ""
		if settings.KeepTree {
			rel = s.RelativeDir
		}
		if err == nil {
			err = validateRelativeDir(rel)
		}
		if err == nil {
			f := getFolder(rel)
			e.Relative = filepath.Join(f.path, name)
			e.Requested = e.Relative
			err = f.err
			if err == nil {
				conflict := func(candidate string) bool {
					k := fold(candidate)
					return f.reserved[k] || f.names[k] != ""
				}
				if conflict(name) {
					if settings.Conflict == Skip {
						e.Skip = true
					} else {
						k := fold(name)
						for i := max(1, f.next[k]); ; i++ {
							if i > 100_000 {
								err = l10n.New("error.tooManyNameCollisions")
								break
							}
							if ctx.Err() != nil {
								return p, ctx.Err()
							}
							candidate := numbered(name, i)
							if !conflict(candidate) {
								e.Relative = filepath.Join(f.path, candidate)
								f.next[k] = i + 1
								break
							}
						}
					}
				}
				if !e.Skip && err == nil {
					f.reserved[fold(filepath.Base(e.Relative))] = true
				}
			}
		}
		e.Issue = err
		p.Entries = append(p.Entries, e)
	}
	return p, nil
}

func numbered(name string, n int) string {
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext) + " (" + strconv.Itoa(n) + ")" + ext
}
