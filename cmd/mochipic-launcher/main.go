// MochiPic's Windows entry point selects the active runtime and starts it.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"imageflow/internal/winupdate"
)

func main() {
	if err := run(); err != nil {
		reportError(err)
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}

func run() error {
	launcher, err := os.Executable()
	if err != nil {
		return err
	}
	if handled, err := updateCommand(launcher); handled {
		return err
	}
	store, err := os.OpenRoot(filepath.Dir(launcher))
	if err != nil {
		return err
	}
	defer store.Close()
	release, err := lockRuntime(store)
	if err != nil {
		return err
	}
	defer release()
	app, err := winupdate.Current(store)
	if err != nil {
		return fmt.Errorf("locate MochiPic program: %w", err)
	}
	for {
		err := runApplication(app)
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != winupdate.RestartExit {
			return err
		}
		next, selectErr := winupdate.Current(store)
		if selectErr != nil {
			return selectErr
		}
		if next == app {
			return err
		}
		app = next
	}
}

func runApplication(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout = os.Stdin, os.Stdout
	if diagnosticInvocation() {
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	var details startupLog
	cmd.Stderr = &details
	err := cmd.Run()
	if cause := strings.TrimSpace(strings.ToValidUTF8(string(details), "�")); err != nil && cause != "" {
		return fmt.Errorf("%w\n\n%s", err, cause)
	}
	return err
}

func diagnosticInvocation() bool {
	if len(os.Args) < 2 {
		return false
	}
	arg := os.Args[1]
	return arg == "--verify-window" || arg == "--verify-runtime" || strings.HasPrefix(arg, "--update-")
}

// Keep the last part of stderr so a GUI launch can show the native failure
// even when Windows did not supply a console handle.
type startupLog []byte

func (log *startupLog) Write(data []byte) (int, error) {
	const limit = 2048
	n := len(data)
	data = data[max(0, n-limit):]
	discard := max(0, len(*log)+len(data)-limit)
	retained := copy(*log, (*log)[discard:])
	*log = append((*log)[:retained], data...)
	return n, nil
}
