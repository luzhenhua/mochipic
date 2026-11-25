// Package batch imports inputs and executes export plans with bounded concurrency.
package batch

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
)

const MaxFiles = 10_000

type ScanOptions struct {
	Recursive bool
	// Exclude skips output folders encountered below an explicitly selected
	// directory. A directly selected file or folder can be imported again.
	Exclude []string
}

type ScanReport struct {
	Added                       []files.Source
	Duplicates, Ignored, Errors int
	Issues                      []ScanIssue
	Limited                     bool
	SkippedOutputFolder         bool
}

type ScanIssue struct {
	Path string
	Err  error
}

type fileID struct{ device, inode uint64 }

func Scan(ctx context.Context, paths []string, existing []files.Source, opts ScanOptions) ScanReport {
	var out ScanReport
	names, ids := map[string]bool{}, map[fileID]bool{}
	for _, item := range existing {
		if ctx.Err() != nil {
			return out
		}
		names[item.Path] = true
		if st, err := os.Lstat(item.Path); err == nil {
			if id := identify(item.Path, st); id.inode != 0 {
				ids[id] = true
			}
		}
	}
	report := func(path string, err error) {
		out.Errors++
		if len(out.Issues) < 20 {
			out.Issues = append(out.Issues, ScanIssue{Path: path, Err: err})
		}
	}
	add := func(path, root string, st fs.FileInfo) {
		if !st.Mode().IsRegular() {
			out.Ignored++
			return
		}
		format := imaging.FormatOf(path)
		if format == "" {
			if root == "" {
				report(path, l10n.New("error.unsupportedInput"))
			} else {
				out.Ignored++
			}
			return
		}
		id := identify(path, st)
		if names[path] || id.inode != 0 && ids[id] {
			out.Duplicates++
			return
		}
		if len(existing)+len(out.Added) >= MaxFiles {
			out.Limited = true
			return
		}
		info, err := imaging.InspectSource(ctx, path, format)
		if err != nil {
			if ctx.Err() == nil {
				report(path, err)
			}
			return
		}
		rel := ""
		if root != "" {
			if parent, err := filepath.Rel(root, filepath.Dir(path)); err == nil && parent != "." {
				rel = parent
			}
		}
		out.Added = append(out.Added, files.Source{Path: path, RelativeDir: rel, Size: st.Size(), Ordinal: len(existing) + len(out.Added), Series: info})
		names[path] = true
		if id.inode != 0 {
			ids[id] = true
		}
	}

	for _, p := range paths {
		if ctx.Err() != nil || out.Limited {
			break
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			report(p, err)
			continue
		}
		st, err := os.Lstat(abs)
		if err != nil {
			report(abs, err)
			continue
		}
		if st.Mode()&os.ModeSymlink != 0 {
			out.Ignored++
			continue
		}
		path, err := filepath.EvalSymlinks(abs)
		if err != nil {
			report(abs, err)
			continue
		}
		if !st.IsDir() {
			add(path, "", st)
			continue
		}
		var nestedOutputs []string
		for _, output := range opts.Exclude {
			if files.Within(output, path) && !files.Within(path, output) {
				nestedOutputs = append(nestedOutputs, output)
			}
		}
		err = filepath.WalkDir(path, func(name string, d fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if out.Limited {
				return fs.SkipAll
			}
			if walkErr != nil {
				report(name, walkErr)
				return nil
			}
			if name == path {
				return nil
			}
			if len(d.Name()) > 0 && d.Name()[0] == '.' {
				out.Ignored++
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				for _, output := range nestedOutputs {
					if files.Within(name, output) {
						out.Ignored++
						out.SkippedOutputFolder = true
						return fs.SkipDir
					}
				}
				if !opts.Recursive {
					return fs.SkipDir
				}
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 || imaging.FormatOf(name) == "" {
				out.Ignored++
				return nil
			}
			if st, e := d.Info(); e != nil {
				report(name, e)
			} else {
				add(name, path, st)
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			report(path, err)
		}
	}
	return out
}
