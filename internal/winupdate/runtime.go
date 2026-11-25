// Package winupdate activates a verified Windows runtime without changing
// files inside any previously installed runtime.
package winupdate

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	activeFile    = "current-runtime"
	runtimePrefix = "runtime-"
	appExecutable = "MochiPic.exe"
)

// Current returns the executable named by the installation's active record.
// The caller opens the private runtime store through os.OpenRoot.
func Current(store *os.Root) (string, error) {
	id, err := currentID(store)
	if err != nil {
		return "", err
	}
	if err := checkExecutable(store, id); err != nil {
		return "", err
	}
	return filepath.Join(store.Name(), id, appExecutable), nil
}

// Install creates a new runtime from files already authenticated by the
// updater. Until the complete runtime is on disk and its active record is
// committed, the previous runtime remains selected. The caller serializes
// activations for the same store.
func Install(ctx context.Context, store *os.Root, verified fs.FS) (string, error) {
	if _, err := currentID(store); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	id := runtimePrefix + rand.Text()
	if err := store.Mkdir(id, 0o700); err != nil {
		return "", err
	}
	committed := false
	defer func() {
		if !committed {
			// The freshly created directory contains only staging files.
			_ = store.RemoveAll(id)
		}
	}()
	runtime, err := store.OpenRoot(id)
	if err != nil {
		return "", err
	}
	err = errors.Join(copyRuntime(ctx, runtime, verified), runtime.Close())
	if err != nil {
		return "", err
	}
	if err := checkExecutable(store, id); err != nil {
		return "", err
	}
	if err := activate(ctx, store, id); err != nil {
		return "", err
	}
	committed = true
	return filepath.Join(store.Name(), id, appExecutable), nil
}

func copyRuntime(ctx context.Context, dest *os.Root, source fs.FS) error {
	return fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:<>\"|?*") {
			return fmt.Errorf("invalid runtime path %q", name)
		}
		local := filepath.FromSlash(name)
		if !filepath.IsLocal(local) {
			return fmt.Errorf("non-local runtime path %q", name)
		}
		if entry.IsDir() {
			return dest.Mkdir(local, 0o700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("runtime entry %q is not a regular file", name)
		}
		input, err := source.Open(name)
		if err != nil {
			return err
		}
		defer input.Close()
		out, err := dest.OpenFile(local, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, input)
		if err == nil {
			err = out.Sync()
		}
		return errors.Join(err, out.Close())
	})
}

func currentID(store *os.Root) (string, error) {
	info, err := store.Lstat(activeFile)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 128 {
		return "", errors.New("invalid active runtime record")
	}
	f, err := store.Open(activeFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 129))
	if err != nil {
		return "", err
	}
	id := strings.TrimSuffix(string(data), "\n")
	if !validRuntimeID(id) {
		return "", errors.New("invalid active runtime name")
	}
	return id, nil
}

func validRuntimeID(id string) bool {
	token, ok := strings.CutPrefix(id, runtimePrefix)
	return ok && len(token) >= 26 && len(token) <= 100 && !strings.ContainsFunc(token, func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '2' || r > '7')
	})
}

func checkExecutable(store *os.Root, id string) error {
	dir, err := store.Lstat(id)
	if err != nil {
		return err
	}
	if !dir.IsDir() {
		return errors.New("invalid runtime directory")
	}
	exe, err := store.Lstat(filepath.Join(id, appExecutable))
	if err != nil {
		return err
	}
	if !exe.Mode().IsRegular() || exe.Size() == 0 {
		return errors.New("invalid runtime executable")
	}
	return nil
}

func activate(ctx context.Context, store *os.Root, id string) error {
	name := ".activate-" + id
	f, err := store.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer store.Remove(name)
	_, err = f.WriteString(id + "\n")
	if err == nil {
		err = f.Sync()
	}
	if err := errors.Join(err, f.Close()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return store.Rename(name, activeFile)
}
