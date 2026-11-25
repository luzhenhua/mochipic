//go:build darwin && arm64

// Package releaseqa tests MyGo against the artifacts of the real packager.
package releaseqa

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

func TestPackagedUpdater(t *testing.T) {
	release := os.Getenv("MOCHIPIC_VERIFY_RELEASE")
	if release == "" {
		t.Skip("set MOCHIPIC_VERIFY_RELEASE to a packaged release directory")
	}
	data, err := os.ReadFile(filepath.Join(release, "update-darwin-arm64.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version   string `json:"version"`
		URL       string `json:"url"`
		Size      int    `json:"size"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(manifest.URL, "https://github.com/luzhenhua/mochipic/releases/download/v"+manifest.Version+"/") {
		t.Fatalf("unexpected release URL: %s", manifest.URL)
	}
	archive, err := os.ReadFile(filepath.Join(release, filepath.Base(manifest.URL)))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive) != manifest.Size || manifest.Version == "" {
		t.Fatal("invalid archive length or version")
	}
	corrupt := append([]byte(nil), archive...)
	corrupt[len(corrupt)/2] ^= 1
	var tamper atomic.Bool
	var downloads atomic.Int32
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	manifest.URL = server.URL + "/archive"
	mux.HandleFunc("/manifest", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(manifest)
	})
	mux.HandleFunc("/archive", func(w http.ResponseWriter, _ *http.Request) {
		downloads.Add(1)
		body := archive
		if tamper.Load() {
			body = corrupt
		}
		_, _ = w.Write(body)
	})

	// This bundle is disposable; the downloaded archive is the actual
	// signed app. The running test is never the user's installation.
	app := filepath.Join(t.TempDir(), "Update Target.app")
	bin := filepath.Join(app, "Contents", "MacOS", "MochiPic")
	if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile("../../scripts/update-public-key.txt")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	flags := "-X github.com/egoist/mygo.production=1 -X github.com/egoist/mygo.packageUpdateFeed=" + server.URL +
		"/manifest -X github.com/egoist/mygo.packageUpdateKey=" + strings.TrimSpace(string(key))
	if out, err := exec.CommandContext(ctx, "go", "test", "-c", "-ldflags", flags, "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build update client: %v\n%s", err, out)
	}
	before := fileDigest(t, bin)
	run := func(mode, version string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, bin, "-test.run", "^TestUpdateClient$")
		cmd.Env = append(os.Environ(), "MOCHIPIC_UPDATE_CASE="+mode, "MOCHIPIC_UPDATE_VERSION="+version)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", mode, err, out)
		}
	}
	run("no-update", manifest.Version)
	run("no-update", "999999.0.0")
	if downloads.Load() != 0 {
		t.Fatal("up-to-date client requested an archive")
	}
	t.Log("current and newer clients did not download an update")

	tamper.Store(true)
	run("tampered", "0.0.0")
	if fileDigest(t, bin) != before {
		t.Fatal("tampered archive changed the running bundle")
	}
	t.Log("modified archive was rejected; test bundle unchanged")

	oldInfo, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	tamper.Store(false)
	run("install", "0.0.0")
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if info, err := os.Stat(bin); err == nil && !os.SameFile(oldInfo, info) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("staged bundle was not swapped after exit")
		case <-ticker.C:
		}
	}
	if out, err := exec.CommandContext(ctx, "codesign", "--verify", "--deep", "--strict", app).CombinedOutput(); err != nil {
		t.Fatalf("installed app signature: %v\n%s", err, out)
	}
	if want := zipDigest(t, filepath.Join(release, "MochiPic-"+manifest.Version+"-arm64.zip")); fileDigest(t, bin) != want {
		t.Fatal("installed executable differs from the manual install ZIP")
	}
	t.Logf("installed signed %s after exit; matches ZIP and passes macOS code signature check", manifest.Version)
}

func TestUpdateClient(t *testing.T) {
	mode := os.Getenv("MOCHIPIC_UPDATE_CASE")
	if mode == "" {
		t.Skip("child of TestPackagedUpdater")
	}
	mygo.App.SetVersion(os.Getenv("MOCHIPIC_UPDATE_VERSION"))
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	u, err := mygo.Updater.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mode == "no-update" {
		if u != nil {
			t.Fatalf("updater offered %s", u.Version)
		}
		return
	}
	if u == nil {
		t.Fatal("no newer version offered")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	before := fileDigest(t, exe)
	err = u.Install(ctx, nil)
	switch mode {
	case "tampered":
		if err == nil || !strings.Contains(err.Error(), "not signed with the app's key") {
			t.Fatalf("expected signature rejection, got %v", err)
		}
	case "install":
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unexpected update case %q", mode)
	}
	if fileDigest(t, exe) != before {
		t.Fatal("the running executable was replaced before it exited")
	}
}

func fileDigest(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return digest(t, f)
}

func zipDigest(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	f, err := z.Open("MochiPic.app/Contents/MacOS/MochiPic")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return digest(t, f)
}

func digest(t *testing.T, r io.Reader) [sha256.Size]byte {
	t.Helper()
	sum := sha256.New()
	if _, err := io.Copy(sum, r); err != nil {
		t.Fatal(err)
	}
	return [sha256.Size]byte(sum.Sum(nil))
}
