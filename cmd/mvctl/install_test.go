package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBinaryNameAndDefaultDir(t *testing.T) {
	name := binaryName()
	if runtime.GOOS == "windows" && name != "mvctl.exe" {
		t.Fatalf("windows binary name: %s", name)
	}
	if runtime.GOOS != "windows" && name != "mvctl" {
		t.Fatalf("unix binary name: %s", name)
	}
	if defaultInstallDir() == "" {
		t.Fatal("default install dir is empty")
	}
	if !strings.Contains(filepath.ToSlash(defaultInstallDir()), ".local/bin") {
		t.Fatalf("unexpected default dir: %s", defaultInstallDir())
	}
}

func TestInstallTarget(t *testing.T) {
	dir := t.TempDir()
	target, err := installTarget(Context{Flags: map[string]string{"dir": dir}})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(target) != binaryName() || filepath.Dir(target) != filepath.Clean(dir) {
		t.Fatalf("target: %s", target)
	}
	if !filepath.IsAbs(target) {
		t.Fatal("target must be absolute")
	}
}

func TestDirOnPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	path := "/usr/bin" + sep + "/home/u/.local/bin" + sep + "/opt/bin"
	if !dirOnPath("/home/u/.local/bin", path) {
		t.Fatal("dir not found on PATH")
	}
	if dirOnPath("/home/u/other", path) {
		t.Fatal("unexpected match")
	}
	if runtime.GOOS == "windows" {
		if !dirOnPath(`C:\Users\U\.local\bin`, `C:\Windows;C:\Users\U\.LOCAL\BIN`) {
			t.Fatal("Windows PATH comparison must be case-insensitive")
		}
	}
}

func TestSameFile(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !sameFile(self, self) {
		t.Fatal("a path must equal itself")
	}
	if sameFile(self, filepath.Join(t.TempDir(), binaryName())) {
		t.Fatal("distinct paths must not match")
	}
}

func TestInstallPreviewDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	ctx := Context{
		G:     Global{Yes: false},
		Flags: map[string]string{"dir": dir},
	}
	r, err := cmdInstall(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Pretty, "Preview only") {
		t.Fatalf("expected a preview: %s", r.Pretty)
	}
	if _, err := os.Stat(filepath.Join(dir, binaryName())); !os.IsNotExist(err) {
		t.Fatal("preview must not write any file")
	}
}

func TestInstallYesCopiesBinary(t *testing.T) {
	dir := t.TempDir()
	ctx := Context{
		G:     Global{Yes: true},
		Flags: map[string]string{"dir": dir},
	}
	r, err := cmdInstall(ctx)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, binaryName())
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("binary not installed: %v\n%s", err, r.Pretty)
	}
	if !strings.Contains(r.Pretty, "installed") {
		t.Fatalf("unexpected output: %s", r.Pretty)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	ctx := Context{G: Global{Yes: true}, Flags: map[string]string{"dir": dir}}
	if _, err := cmdInstall(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := cmdInstall(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Pretty, "already installed") {
		t.Fatalf("expected idempotent result: %s", r.Pretty)
	}
	changed, _ := r.Data.(map[string]any)["changed"]
	if changed != false {
		t.Fatal("must report changed=false")
	}
}

func TestUninstallGuards(t *testing.T) {
	dir := t.TempDir()
	ctx := Context{G: Global{Yes: false}, Flags: map[string]string{"dir": dir}}
	r, err := cmdUninstall(ctx)
	if err != nil || !strings.Contains(r.Pretty, "No installed copy") {
		t.Fatalf("unexpected: %s %v", r.Pretty, err)
	}
	// An installed copy in a temp dir must not be removed without --yes.
	target := filepath.Join(dir, binaryName())
	if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err = cmdUninstall(ctx)
	if err != nil || !strings.Contains(r.Pretty, "Preview only") {
		t.Fatalf("expected preview: %s %v", r.Pretty, err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("preview removed the file")
	}
}
