package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"imageflow/internal/winupdate"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"
)

const (
	stagePrefix  = "mochipic-update-"
	verifierName = "UpdateVerifier.exe"
)

func updateCommand(exe string) (handled bool, err error) {
	if len(os.Args) < 2 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	switch os.Args[1] {
	case winupdate.ApplyCommand:
		if len(os.Args) != 3 {
			return true, errors.New("expected an update version")
		}
		return true, applyUpdate(ctx, exe, os.Args[2])
	case winupdate.VerifyCommand:
		if len(os.Args) != 4 {
			return true, errors.New("expected current and requested update versions")
		}
		return true, verifyUpdate(ctx, exe, os.Args[2], os.Args[3])
	case winupdate.CleanupCommand:
		if len(os.Args) != 2 {
			return true, errors.New("unexpected uninstall parameters")
		}
		return true, cleanupInstallation(exe)
	default:
		return false, nil
	}
}

func applyUpdate(ctx context.Context, launcher, expected string) error {
	store, err := os.OpenRoot(filepath.Dir(launcher))
	if err != nil {
		return err
	}
	defer store.Close()
	lock, err := acquireLock(store, winupdate.UpdateLockFile, true)
	if err != nil {
		return err
	}
	defer releaseLock(lock)
	active, err := winupdate.Current(store)
	if err != nil {
		return err
	}
	current, err := winupdate.ReleaseVersion(os.DirFS(filepath.Dir(active)))
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp("", stagePrefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	verifier := filepath.Join(stage, verifierName)
	if err := copyVerifier(launcher, verifier); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, verifier, winupdate.VerifyCommand, current, expected)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("authenticate the Windows update: %w", err)
	}
	if err := os.Remove(verifier); err != nil {
		return err
	}
	// MyGo uses this file only for its in-place staging swap. It is not
	// part of the installed runtime or its ownership receipt.
	if err := os.Remove(filepath.Join(stage, ".mygo-files")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	verified := os.DirFS(stage)
	version, err := winupdate.ReleaseVersion(verified)
	if err != nil || version != expected {
		return fmt.Errorf("authenticated runtime does not match update version %q", expected)
	}
	// Load the new executable and its private codecs while the previous
	// runtime remains active. A broken DLL set never becomes the startup target.
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	probe := exec.CommandContext(probeCtx, filepath.Join(stage, "MochiPic.exe"), "--verify-runtime")
	probe.Dir = stage
	probe.Env = append(os.Environ(), "VIPSHOME=", "FONTCONFIG_FILE=", "MAGICK_CONFIGURE_PATH=")
	if output, err := probe.CombinedOutput(); err != nil {
		return fmt.Errorf("start the authenticated runtime: %w: %s", err, output)
	}
	_, err = winupdate.Install(ctx, store, verified)
	return err
}

func lockRuntime(store *os.Root) (func(), error) {
	lock, err := acquireLock(store, winupdate.RuntimeLockFile, false)
	if err != nil {
		return nil, err
	}
	return func() { _ = releaseLock(lock) }, nil
}

func cleanupInstallation(launcher string) error {
	store, err := os.OpenRoot(filepath.Dir(launcher))
	if err != nil {
		return err
	}
	defer store.Close()
	runtime, err := acquireLock(store, winupdate.RuntimeLockFile, true)
	if err != nil {
		return err
	}
	update, err := acquireLock(store, winupdate.UpdateLockFile, true)
	if err != nil {
		_ = releaseLock(runtime)
		return err
	}
	if err := errors.Join(winupdate.UninstallRuntimes(store), releaseLock(update), releaseLock(runtime)); err != nil {
		return err
	}
	return winupdate.RemoveEmptyLocks(store)
}

func acquireLock(store *os.Root, path string, exclusive bool) (*os.File, error) {
	info, err := store.Lstat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if info != nil && (!info.Mode().IsRegular() || info.Size() != 0) {
		return nil, fmt.Errorf("invalid coordination file %q", path)
	}
	mode := os.O_RDONLY
	if path == winupdate.UpdateLockFile || exclusive && errors.Is(err, fs.ErrNotExist) {
		mode = os.O_CREATE | os.O_RDWR
	}
	file, err := store.OpenFile(path, mode, 0o600)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() != 0 || info != nil && !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, fmt.Errorf("coordination file %q changed", path)
	}
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	var offset windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, &offset); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("MochiPic is using the installation directory: %w", err)
	}
	return file, nil
}

func releaseLock(file *os.File) error {
	var offset windows.Overlapped
	return errors.Join(windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &offset), file.Close())
}

func copyVerifier(launcher, verifier string) error {
	in, err := os.Open(launcher)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(verifier, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

func verifyUpdate(ctx context.Context, exe, current, expected string) error {
	stage := filepath.Dir(exe)
	if !strings.EqualFold(filepath.Base(exe), verifierName) || !strings.HasPrefix(filepath.Base(stage), stagePrefix) {
		return errors.New("updater must run from an isolated staging directory")
	}
	stageParent, err := os.Stat(filepath.Dir(stage))
	if err != nil {
		return err
	}
	temp, err := os.Stat(os.TempDir())
	if err != nil || !os.SameFile(stageParent, temp) {
		return errors.New("update stage must be inside the system temporary directory")
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) != 1 || !entries[0].Type().IsRegular() || !strings.EqualFold(entries[0].Name(), verifierName) {
		return errors.New("update staging directory must contain only the verifier")
	}
	mygo.App.SetVersion(current)
	update, err := mygo.Updater.Check(ctx)
	if err != nil {
		return err
	}
	if update == nil || update.Version != expected {
		return errors.New("requested update is no longer available")
	}
	return update.Install(ctx, nil)
}
