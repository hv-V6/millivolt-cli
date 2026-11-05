package main

// Self-install: put the running mvctl binary on the user's PATH. Standard
// library only, standalone-only (the small upstream contribution needs no
// installer). No shell scripts, no shims, no package manager.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func InstallCommands() []Command {
	return []Command{
		{Name: "install", Group: "manage", Usage: "mvctl install [--dir PATH] --yes", Summary: "Install mvctl as a normal command", Run: cmdInstall},
		{Name: "uninstall", Group: "manage", Usage: "mvctl uninstall [--dir PATH] --yes", Summary: "Remove the installed copy", Run: cmdUninstall},
	}
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "mvctl.exe"
	}
	return "mvctl"
}

func defaultInstallDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "bin")
}

func installTarget(ctx Context) (string, error) {
	dir := flag(ctx, "dir", "")
	if dir == "" {
		dir = defaultInstallDir()
	}
	if dir == "" {
		return "", fmt.Errorf("could not determine a home directory; pass --dir PATH")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(abs, binaryName()), nil
}

func pathEnv() string {
	key := "PATH"
	if runtime.GOOS == "windows" {
		// Windows is case-insensitive; either spelling can be present.
		if v := os.Getenv("Path"); v != "" {
			return v
		}
	}
	return os.Getenv(key)
}

func dirOnPath(dir, path string) bool {
	want := strings.ToLower(filepath.Clean(dir))
	for _, entry := range strings.Split(path, string(os.PathListSeparator)) {
		if entry == "" {
			continue
		}
		if strings.ToLower(filepath.Clean(entry)) == want {
			return true
		}
	}
	return false
}

func cmdInstall(ctx Context) (Result, error) {
	target, err := installTarget(ctx)
	if err != nil {
		return Result{Code: 1}, err
	}
	self, err := os.Executable()
	if err != nil {
		return Result{Code: 1}, err
	}
	selfAbs, _ := filepath.Abs(self)
	targetDir := filepath.Dir(target)
	onPath := dirOnPath(targetDir, pathEnv())

	if sameFile(selfAbs, target) || sameContent(selfAbs, target) {
		return Result{
			Pretty: green("mvctl is already installed at ") + target + "\n" +
				dim("Run 'mvctl status' from any terminal."),
			Data: map[string]any{"installed": true, "path": target, "on_path": onPath, "changed": false},
		}, nil
	}
	if !wantYes(ctx) {
		preview := heading("install preview") + "\n" + kv([][2]string{
			{"source", selfAbs},
			{"target", target},
			{"on PATH", boolText(onPath)},
		}) + "\n\n" + yellow("Preview only. Re-run with --yes to install.")
		if !onPath {
			preview += "\n" + yellow(fmt.Sprintf("Add %s to your PATH afterwards.", targetDir))
		}
		return Result{Pretty: preview, Data: map[string]any{"installed": false, "target": target, "on_path": onPath, "changed": false}}, nil
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return Result{Code: 1}, err
	}
	if err := copyExecutable(selfAbs, target); err != nil {
		return Result{Code: 1}, err
	}
	pretty := green("mvctl installed at ") + target + "\n"
	if onPath {
		pretty += green("It is on your PATH: ") + dim("mvctl status")
	} else {
		pretty += yellow("Not on PATH yet. Add this directory:") + "\n  " + targetDir
	}
	return Result{Pretty: pretty, Data: map[string]any{"installed": true, "target": target, "on_path": onPath, "changed": true}}, nil
}

func cmdUninstall(ctx Context) (Result, error) {
	target, err := installTarget(ctx)
	if err != nil {
		return Result{Code: 1}, err
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return Result{Pretty: dim("No installed copy at ") + target, Data: map[string]any{"removed": false, "path": target}}, nil
	}
	if !wantYes(ctx) {
		return Result{Pretty: yellow("Preview only. Re-run with --yes to remove ") + target,
			Data: map[string]any{"removed": false, "path": target}}, nil
	}
	self, _ := os.Executable()
	if sameFile(self, target) {
		return Result{Code: 2}, fmt.Errorf("cannot uninstall the binary you are running; delete %s manually", target)
	}
	if err := os.Remove(target); err != nil {
		return Result{Code: 1}, err
	}
	return Result{Pretty: green("removed ") + target, Data: map[string]any{"removed": true, "path": target}}, nil
}

func sameFile(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
}

// sameContent makes a re-run idempotent even when the binary was installed
// from a different path (for example from a build directory into ~/.local/bin).
func sameContent(a, b string) bool {
	ha, err := fileHash(a)
	if err != nil {
		return false
	}
	hb, err := fileHash(b)
	if err != nil {
		return false
	}
	return ha == hb
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmp, 0o755)
	}
	// A rename over an existing Windows executable can fail while it is in
	// use; remove the old copy first, then rename.
	if _, err := os.Stat(dst); err == nil {
		if err := os.Remove(dst); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("could not replace %s: %w", dst, err)
		}
	}
	return os.Rename(tmp, dst)
}
