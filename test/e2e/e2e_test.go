package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var envoyageBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "envoyage-e2e-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	envoyageBin = filepath.Join(dir, "envoyage")
	if runtime.GOOS == "windows" {
		envoyageBin += ".exe"
	}

	repoRoot := filepath.Join("..", "..")
	cmd := exec.Command("go", "build", "-ldflags", "-X main.version=e2e", "-o", envoyageBin, "./cmd/envoyage")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestComposeLoadsDefaultEnvAndEncryptedSecrets(t *testing.T) {
	requirePOSIX(t)

	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	captureDir := filepath.Join(dir, "capture")
	runtimePath := filepath.Join(dir, "docker")
	writeFakeRuntime(t, runtimePath)
	mkdirAll(t, appDir)
	mkdirAll(t, captureDir)

	writeFile(t, filepath.Join(appDir, ".env"), "APP_ENV=e2e\nTOKEN=from-plain-env\n")
	writeFile(t, filepath.Join(appDir, ".secrets.env"), "TOKEN=from-encrypted-env\nDB_PASSWORD=super-secret\n")
	runEnvoyage(t, appDir, []string{"keygen", "--out", "age-key.txt"}, nil)
	runEnvoyage(t, appDir, []string{"encrypt"}, map[string]string{"AGE_IDENTITY_FILE": "./age-key.txt"})
	removeFile(t, filepath.Join(appDir, ".secrets.env"))

	stdout, stderr, err := runEnvoyageOutput(t, appDir, []string{"compose", "config"}, map[string]string{
		"AGE_IDENTITY_FILE":   "./age-key.txt",
		"ENVOYAGE_DOCKER_BIN": runtimePath,
		"CAPTURE_DIR":         captureDir,
	})
	if err != nil {
		t.Fatalf("envoyage compose error = %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	assertLines(t, filepath.Join(captureDir, "args"), []string{"compose", "config"})
	assertFile(t, filepath.Join(captureDir, "APP_ENV"), "e2e\n")
	assertFile(t, filepath.Join(captureDir, "TOKEN"), "from-encrypted-env\n")
	assertFile(t, filepath.Join(captureDir, "DB_PASSWORD"), "super-secret\n")
	assertNotContains(t, stdout, "super-secret")
	assertNotContains(t, stderr, "super-secret")
	if _, err := os.Stat(filepath.Join(appDir, ".secrets.env")); !os.IsNotExist(err) {
		t.Fatalf(".secrets.env should not be recreated, stat error = %v", err)
	}
}

func TestDockerAndPodmanShimInstallAndCompose(t *testing.T) {
	requirePOSIX(t)

	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	binDir := filepath.Join(dir, "bin")
	realDir := filepath.Join(dir, "real")
	appDir := filepath.Join(dir, "app")
	captureDir := filepath.Join(dir, "capture")
	mkdirAll(t, homeDir)
	mkdirAll(t, realDir)
	mkdirAll(t, appDir)
	mkdirAll(t, captureDir)

	writeFakeRuntime(t, filepath.Join(realDir, "docker"))
	writeFakeRuntime(t, filepath.Join(realDir, "podman"))
	writeFile(t, filepath.Join(appDir, ".env"), "APP_ENV=shim-e2e\n")
	writeFile(t, filepath.Join(appDir, ".secrets.env"), "TOKEN=shim-secret\n")
	runEnvoyage(t, appDir, []string{"keygen", "--out", "age-key.txt"}, nil)
	runEnvoyage(t, appDir, []string{"encrypt"}, map[string]string{"AGE_IDENTITY_FILE": "./age-key.txt"})
	removeFile(t, filepath.Join(appDir, ".secrets.env"))

	installStdout, installStderr, err := runEnvoyageOutput(t, dir, []string{"shim", "install", "--runtime", "all", "--bin-dir", binDir}, map[string]string{
		"HOME": homeDir,
		"PATH": realDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	if err != nil {
		t.Fatalf("shim install error = %v\nstdout=%s\nstderr=%s", err, installStdout, installStderr)
	}
	assertSymlink(t, filepath.Join(binDir, "docker"))
	assertSymlink(t, filepath.Join(binDir, "podman"))
	assertContains(t, installStdout, "installed docker shim")
	assertContains(t, installStdout, "installed podman shim")

	runRuntime(t, appDir, "docker", []string{"compose", "config"}, map[string]string{
		"PATH":              binDir + string(os.PathListSeparator) + realDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"AGE_IDENTITY_FILE": "./age-key.txt",
		"CAPTURE_DIR":       captureDir,
	})
	assertFile(t, filepath.Join(captureDir, "runtime"), "docker\n")
	assertLines(t, filepath.Join(captureDir, "args"), []string{"compose", "config"})
	assertFile(t, filepath.Join(captureDir, "TOKEN"), "shim-secret\n")

	runRuntime(t, appDir, "podman", []string{"compose", "config"}, map[string]string{
		"PATH":              binDir + string(os.PathListSeparator) + realDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"AGE_IDENTITY_FILE": "./age-key.txt",
		"CAPTURE_DIR":       captureDir,
	})
	assertFile(t, filepath.Join(captureDir, "runtime"), "podman\n")
	assertLines(t, filepath.Join(captureDir, "args"), []string{"compose", "config"})
	assertFile(t, filepath.Join(captureDir, "TOKEN"), "shim-secret\n")
}

func TestShimPassesThroughNonComposeWithoutLoadingSecrets(t *testing.T) {
	requirePOSIX(t)

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	realDir := filepath.Join(dir, "real")
	appDir := filepath.Join(dir, "app")
	captureDir := filepath.Join(dir, "capture")
	mkdirAll(t, binDir)
	mkdirAll(t, realDir)
	mkdirAll(t, appDir)
	mkdirAll(t, captureDir)

	if err := os.Symlink(envoyageBin, filepath.Join(binDir, "docker")); err != nil {
		t.Fatalf("Symlink(docker shim) error = %v", err)
	}
	writeFakeRuntime(t, filepath.Join(realDir, "docker"))
	writeFile(t, filepath.Join(appDir, ".env"), "TOKEN=should-not-load\n")

	runRuntime(t, appDir, "docker", []string{"ps", "--all"}, map[string]string{
		"PATH":        binDir + string(os.PathListSeparator) + realDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"CAPTURE_DIR": captureDir,
		"TOKEN":       "",
	})

	assertLines(t, filepath.Join(captureDir, "args"), []string{"ps", "--all"})
	assertFile(t, filepath.Join(captureDir, "TOKEN"), "\n")
}

func runEnvoyage(t *testing.T, dir string, args []string, env map[string]string) {
	t.Helper()
	stdout, stderr, err := runEnvoyageOutput(t, dir, args, env)
	if err != nil {
		t.Fatalf("envoyage %s error = %v\nstdout=%s\nstderr=%s", strings.Join(args, " "), err, stdout, stderr)
	}
}

func runEnvoyageOutput(t *testing.T, dir string, args []string, env map[string]string) (string, string, error) {
	t.Helper()
	return runCommand(t, dir, envoyageBin, args, env)
}

func runRuntime(t *testing.T, dir string, name string, args []string, env map[string]string) {
	t.Helper()
	stdout, stderr, err := runCommand(t, dir, lookPath(t, name, env), args, env)
	if err != nil {
		t.Fatalf("%s %s error = %v\nstdout=%s\nstderr=%s", name, strings.Join(args, " "), err, stdout, stderr)
	}
}

func runCommand(t *testing.T, dir string, name string, args []string, env map[string]string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergedEnv(env)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func mergedEnv(overrides map[string]string) []string {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}

	env := make([]string, 0, len(values))
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

func lookPath(t *testing.T, name string, env map[string]string) string {
	t.Helper()
	pathValue := env["PATH"]
	if pathValue == "" {
		pathValue = os.Getenv("PATH")
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return candidate
		}
	}
	t.Fatalf("%s not found in PATH %q", name, pathValue)
	return ""
}

func writeFakeRuntime(t *testing.T, path string) {
	t.Helper()
	script := `#!/bin/sh
set -eu
: "${CAPTURE_DIR:?}"
mkdir -p "$CAPTURE_DIR"
basename "$0" > "$CAPTURE_DIR/runtime"
: > "$CAPTURE_DIR/args"
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$CAPTURE_DIR/args"
done
printf '%s\n' "${APP_ENV:-}" > "$CAPTURE_DIR/APP_ENV"
printf '%s\n' "${TOKEN:-}" > "$CAPTURE_DIR/TOKEN"
printf '%s\n' "${DB_PASSWORD:-}" > "$CAPTURE_DIR/DB_PASSWORD"
printf 'fake-runtime-ok\n'
`
	writeFileMode(t, path, script, 0o700)
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	writeFileMode(t, path, data, 0o600)
}

func writeFileMode(t *testing.T, path string, data string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove(%s) error = %v", path, err)
	}
}

func assertFile(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}

func assertLines(t *testing.T, path string, want []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("%s lines = %#v, want %#v", path, got, want)
	}
}

func assertContains(t *testing.T, got string, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("%q does not contain %q", got, want)
	}
}

func assertNotContains(t *testing.T, got string, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("%q unexpectedly contains %q", got, want)
	}
}

func assertSymlink(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(%s) error = %v", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", path)
	}
}

func requirePOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("e2e tests use POSIX shell scripts and symlinks")
	}
}
