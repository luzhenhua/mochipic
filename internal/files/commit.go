package files

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"imageflow/internal/l10n"
)

var ErrCollision = l10n.New("error.fileExists")

type Destination struct {
	root     *os.File
	plan     Plan
	mu       sync.Mutex
	reserved map[string]bool
}

// OpenDestination pins the planned directory and probes actual write access
// before any worker starts. Directories beneath it are opened without symlinks.
func OpenDestination(p Plan) (*Destination, error) {
	if p.Directory == "" || p.rootInfo == nil {
		return nil, l10n.New("error.chooseOutputFolder")
	}
	root, err := openAbsolute(p.Directory)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = root.Close()
		}
	}()
	st, err := root.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(st, p.rootInfo) {
		return nil, l10n.New("error.outputFolderChanged")
	}
	name, f, err := tempAt(root)
	if err != nil {
		return nil, l10n.Wrap("error.outputFolderNotWritable", err)
	}
	if err := closeTemp(root, name, f, false); err != nil {
		return nil, l10n.Wrap("error.cleanupWriteCheck", err)
	}
	d := &Destination{root: root, plan: p, reserved: make(map[string]bool, len(p.Entries))}
	for _, e := range p.Entries {
		if !e.Skip && e.Issue == nil {
			d.reserved[fold(e.Relative)] = true
		}
	}
	ok = true
	return d, nil
}

func (d *Destination) Close() error { return d.root.Close() }

func tempAt(dir *os.File) (string, *os.File, error) {
	for range 8 {
		var buf [12]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return "", nil, err
		}
		name := ".imageflow-" + hex.EncodeToString(buf[:]) + ".tmp"
		f, err := createTemp(dir, name)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		return name, f, nil
	}
	return "", nil, l10n.New("error.noUniqueTempFile")
}

// Commit writes to a temporary sibling, fsyncs it and uses an exclusive
// kernel rename (or exclusive hard link) as the only publication operation.
func (d *Destination) Commit(ctx context.Context, e Entry, write func(io.Writer) error) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if e.Issue != nil {
		return "", e.Issue
	}
	if e.Skip {
		return "", ErrCollision
	}
	rel, base := filepath.Dir(e.Relative), filepath.Base(e.Relative)
	if !validComponent(base) || validateRelativeDir(rel) != nil || filepath.IsAbs(e.Relative) ||
		!d.isReserved(e.Relative) || Within(e.Source.Path, d.plan.Directory) {
		return "", l10n.New("error.unplannedOutputPath")
	}
	dir, err := openTree(d.root, rel, true)
	if err != nil {
		return "", l10n.Wrap("error.openOutputSubfolder", err)
	}
	defer dir.Close()
	id, err := dir.Stat()
	if err != nil {
		return "", err
	}
	tmp, f, err := tempAt(dir)
	if err != nil {
		return "", l10n.Wrap("error.createTempOutput", err)
	}
	published := false
	defer func() { _ = closeTemp(dir, tmp, f, published) }()
	if err = write(f); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", l10n.Wrap("error.writeToDisk", err)
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	current, err := openAbsolute(filepath.Join(d.plan.Directory, rel))
	if err != nil {
		return "", l10n.Wrap("error.outputFolderMovedDetails", err)
	}
	st, err := current.Stat()
	_ = current.Close()
	if err != nil || !os.SameFile(st, id) {
		return "", l10n.New("error.outputFolderMoved")
	}
	names, err := snapshotFolder(dir)
	if err != nil {
		return "", err
	}
	candidate := base
	for i := 0; i < 100_000; i++ {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		target := filepath.Join(rel, candidate)
		if names[fold(candidate)] == "" && (candidate == base || !d.reserved[fold(target)]) {
			err = publishTemp(dir, f, tmp, candidate)
			if err == nil {
				published = true
				return filepath.Join(d.plan.Directory, target), nil
			}
			if !errors.Is(err, os.ErrExist) {
				return "", l10n.Wrap("error.commitOutput", err)
			}
		}
		if d.plan.Conflict == Skip {
			return "", ErrCollision
		}
		candidate = numbered(filepath.Base(e.Requested), i+1)
	}
	return "", l10n.New("error.tooManyNameCollisions")
}

func (d *Destination) isReserved(path string) bool {
	// Reservations are immutable for the lifetime of the batch.
	return d.reserved[fold(path)]
}
