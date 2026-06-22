package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"filippo.io/age"
)

func TestParseArgsRemovesEnvFilesAndPreservesComposeArgs(t *testing.T) {
	opts, err := ParseArgs([]string{"--env-file", ".env.age", "-f", "compose.yaml", "-p", "myapp", "up", "-d"})
	if err != nil {
		t.Fatalf("ParseArgs() error = %v", err)
	}

	if len(opts.EnvFiles) != 1 || opts.EnvFiles[0] != ".env.age" {
		t.Fatalf("EnvFiles = %#v, want [.env.age]", opts.EnvFiles)
	}
	want := []string{"-f", "compose.yaml", "-p", "myapp", "up", "-d"}
	if strings.Join(opts.ComposeArgs, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("ComposeArgs = %#v, want %#v", opts.ComposeArgs, want)
	}
}

func TestParseArgsSupportsEqualsFormsAndIdentityEnvFallback(t *testing.T) {
	t.Setenv("AGE_IDENTITY_FILE", "/env/identity.txt")

	opts, err := ParseArgs([]string{"--env-file=.env", "--env-file=.env.age", "config"})
	if err != nil {
		t.Fatalf("ParseArgs() error = %v", err)
	}
	wantEnvFiles := []string{".env", ".env.age"}
	if strings.Join(opts.EnvFiles, "\x00") != strings.Join(wantEnvFiles, "\x00") {
		t.Fatalf("EnvFiles = %#v, want %#v", opts.EnvFiles, wantEnvFiles)
	}
	if opts.IdentityFile != "/env/identity.txt" {
		t.Fatalf("IdentityFile = %q, want env fallback", opts.IdentityFile)
	}
	if strings.Join(opts.ComposeArgs, "\x00") != "config" {
		t.Fatalf("ComposeArgs = %#v, want [config]", opts.ComposeArgs)
	}
}

func TestParseArgsIdentityFlagOverridesEnv(t *testing.T) {
	t.Setenv("AGE_IDENTITY_FILE", "/env/identity.txt")

	opts, err := ParseArgs([]string{"--identity=/flag/identity.txt", "--env-file", ".env.age", "up"})
	if err != nil {
		t.Fatalf("ParseArgs() error = %v", err)
	}
	if opts.IdentityFile != "/flag/identity.txt" {
		t.Fatalf("IdentityFile = %q, want flag value", opts.IdentityFile)
	}
}

func TestParseArgsUsesDefaultEnvFilesWhenNoneAreExplicit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(".env", []byte("APP_ENV=test\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}
	if err := os.WriteFile(".env.age", []byte("not-used-by-parse\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env.age) error = %v", err)
	}

	opts, err := ParseArgs([]string{"-f", "compose.yaml", "config"})
	if err != nil {
		t.Fatalf("ParseArgs() error = %v", err)
	}
	want := []string{".env", ".env.age"}
	if strings.Join(opts.EnvFiles, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("EnvFiles = %#v, want %#v", opts.EnvFiles, want)
	}
	if strings.Join(opts.ComposeArgs, "\x00") != strings.Join([]string{"-f", "compose.yaml", "config"}, "\x00") {
		t.Fatalf("ComposeArgs = %#v, want compose args", opts.ComposeArgs)
	}
}

func TestParseArgsSkipsMissingDefaultEnvFiles(t *testing.T) {
	t.Chdir(t.TempDir())

	opts, err := ParseArgs([]string{"config"})
	if err != nil {
		t.Fatalf("ParseArgs() error = %v", err)
	}
	if len(opts.EnvFiles) != 0 {
		t.Fatalf("EnvFiles = %#v, want none", opts.EnvFiles)
	}
}

func TestParseArgsExplicitEnvFilesDisableDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(".env", []byte("APP_ENV=test\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	opts, err := ParseArgs([]string{"--env-file", "custom.env", "config"})
	if err != nil {
		t.Fatalf("ParseArgs() error = %v", err)
	}
	want := []string{"custom.env"}
	if strings.Join(opts.EnvFiles, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("EnvFiles = %#v, want %#v", opts.EnvFiles, want)
	}
}

func TestParseArgsUsesDefaultIdentityFile(t *testing.T) {
	t.Setenv("AGE_IDENTITY_FILE", "")
	oldDefault := defaultIdentityFile
	defaultIdentityFile = "/tmp/envoyage-default-key.txt"
	t.Cleanup(func() {
		defaultIdentityFile = oldDefault
	})

	opts, err := ParseArgs([]string{"--env-file", ".env.age", "config"})
	if err != nil {
		t.Fatalf("ParseArgs() error = %v", err)
	}
	if opts.IdentityFile != "/tmp/envoyage-default-key.txt" {
		t.Fatalf("IdentityFile = %q, want default identity", opts.IdentityFile)
	}
}

func TestParseArgsRequiresFlagValues(t *testing.T) {
	tests := [][]string{
		{"--env-file"},
		{"--identity"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := ParseArgs(args)
			if err == nil {
				t.Fatal("ParseArgs() error = nil, want error")
			}
		})
	}
}

func TestLoadEnvFilesLaterFilesOverrideEarlier(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, ".env")
	second := filepath.Join(dir, "override.env")

	if err := os.WriteFile(first, []byte("DB_USER=app\nDB_PASSWORD=old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(first) error = %v", err)
	}
	if err := os.WriteFile(second, []byte("DB_PASSWORD=new\nAPI_KEY=secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(second) error = %v", err)
	}

	got, err := LoadEnvFiles([]string{first, second}, "")
	if err != nil {
		t.Fatalf("LoadEnvFiles() error = %v", err)
	}

	if got["DB_USER"] != "app" {
		t.Fatalf("DB_USER = %q, want app", got["DB_USER"])
	}
	if got["DB_PASSWORD"] != "new" {
		t.Fatalf("DB_PASSWORD = %q, want new", got["DB_PASSWORD"])
	}
	if got["API_KEY"] != "secret" {
		t.Fatalf("API_KEY = %q, want secret", got["API_KEY"])
	}
}

func TestMergeEnvOverlaysValuesAndPreservesExistingOrder(t *testing.T) {
	got := MergeEnv([]string{"A=1", "TOKEN=old", "NO_EQUALS", "B=2"}, map[string]string{
		"TOKEN": "new",
		"EMPTY": "",
	})

	env := envSliceToMap(got)
	if env["A"] != "1" {
		t.Fatalf("A = %q, want 1", env["A"])
	}
	if env["TOKEN"] != "new" {
		t.Fatalf("TOKEN = %q, want new", env["TOKEN"])
	}
	if env["B"] != "2" {
		t.Fatalf("B = %q, want 2", env["B"])
	}
	if env["EMPTY"] != "" {
		t.Fatalf("EMPTY = %q, want empty", env["EMPTY"])
	}

	if got[0] != "A=1" || got[1] != "TOKEN=new" || got[2] != "B=2" {
		t.Fatalf("MergeEnv order prefix = %#v, want A/TOKEN/B", got[:3])
	}
}

func TestLoadEnvFilesDecryptsAgeFileAndRequiresIdentity(t *testing.T) {
	dir := t.TempDir()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity() error = %v", err)
	}
	identityPath := filepath.Join(dir, "identity.txt")
	if err := os.WriteFile(identityPath, []byte(identity.String()+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(identity) error = %v", err)
	}

	encryptedPath := filepath.Join(dir, ".env.age")
	if err := writeEncryptedDotenv(encryptedPath, identity.Recipient(), "TOKEN=secret-token\n"); err != nil {
		t.Fatalf("writeEncryptedDotenv() error = %v", err)
	}

	got, err := LoadEnvFiles([]string{encryptedPath}, identityPath)
	if err != nil {
		t.Fatalf("LoadEnvFiles() error = %v", err)
	}
	if got["TOKEN"] != "secret-token" {
		t.Fatalf("TOKEN = %q, want secret-token", got["TOKEN"])
	}

	oldDefault := defaultIdentityFile
	defaultIdentityFile = filepath.Join(dir, "missing-default-key.txt")
	t.Cleanup(func() {
		defaultIdentityFile = oldDefault
	})

	opts, err := ParseArgs([]string{"--env-file", encryptedPath, "config"})
	if err != nil {
		t.Fatalf("ParseArgs(default identity) error = %v", err)
	}
	_, err = LoadEnvFiles(opts.EnvFiles, opts.IdentityFile)
	if err == nil {
		t.Fatal("LoadEnvFiles() error = nil, want identity error")
	}
	if !strings.Contains(err.Error(), "missing-default-key.txt") {
		t.Fatalf("error = %q, want default identity path", err.Error())
	}
}

func TestLoadEnvFilesUsesDefaultIdentityFromParsedArgs(t *testing.T) {
	dir := t.TempDir()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity() error = %v", err)
	}
	defaultPath := filepath.Join(dir, "envoyage-key.txt")
	if err := os.WriteFile(defaultPath, []byte(identity.String()+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(identity) error = %v", err)
	}
	encryptedPath := filepath.Join(dir, ".env.age")
	if err := writeEncryptedDotenv(encryptedPath, identity.Recipient(), "TOKEN=default-identity\n"); err != nil {
		t.Fatalf("writeEncryptedDotenv() error = %v", err)
	}

	t.Setenv("AGE_IDENTITY_FILE", "")
	oldDefault := defaultIdentityFile
	defaultIdentityFile = defaultPath
	t.Cleanup(func() {
		defaultIdentityFile = oldDefault
	})

	opts, err := ParseArgs([]string{"--env-file", encryptedPath, "config"})
	if err != nil {
		t.Fatalf("ParseArgs() error = %v", err)
	}
	got, err := LoadEnvFiles(opts.EnvFiles, opts.IdentityFile)
	if err != nil {
		t.Fatalf("LoadEnvFiles() error = %v", err)
	}
	if got["TOKEN"] != "default-identity" {
		t.Fatalf("TOKEN = %q, want default-identity", got["TOKEN"])
	}
}

func TestLoadEnvFilesParseErrorDoesNotLeakSecretValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("API_KEY=\"super-secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadEnvFiles([]string{path}, "")
	if err == nil {
		t.Fatal("LoadEnvFiles() error = nil, want parse error")
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("error leaked secret value: %v", err)
	}
	if !strings.Contains(err.Error(), "API_KEY") {
		t.Fatalf("error = %q, want key name", err.Error())
	}
}

func TestRunnerPassesComposeArgsAndDoesNotLeakSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "capture.json")
	scriptPath := filepath.Join(dir, "docker")
	script := `#!/bin/sh
printf '{"args":[' > "$CAPTURE"
first=1
for arg in "$@"; do
  if [ "$first" = 1 ]; then first=0; else printf ',' >> "$CAPTURE"; fi
  printf '"%s"' "$arg" >> "$CAPTURE"
done
printf '],"token":"%s"}' "$TOKEN" >> "$CAPTURE"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	runner := Runner{
		DockerBin: scriptPath,
		Env:       []string{"CAPTURE=" + outputPath},
		Stdout:    &stdout,
		Stderr:    &stderr,
	}

	err := runner.Run(context.Background(), []string{"config"}, map[string]string{"TOKEN": "super-secret"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if strings.Contains(stdout.String(), "super-secret") || strings.Contains(stderr.String(), "super-secret") {
		t.Fatalf("secret leaked to output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}

	var capture struct {
		Args  []string `json:"args"`
		Token string   `json:"token"`
	}
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatalf("Unmarshal(capture) error = %v; data=%s", err, data)
	}

	wantArgs := []string{"compose", "config"}
	if strings.Join(capture.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", capture.Args, wantArgs)
	}
	if capture.Token != "super-secret" {
		t.Fatalf("TOKEN = %q, want super-secret", capture.Token)
	}
}

func TestRunnerForwardsStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "stdin.txt")
	scriptPath := filepath.Join(dir, "docker")
	script := `#!/bin/sh
IFS= read -r line
printf '%s' "$line" > "$CAPTURE"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}

	runner := Runner{
		DockerBin: scriptPath,
		Env:       []string{"CAPTURE=" + outputPath},
		Stdin:     strings.NewReader("hello-from-stdin\n"),
		Stdout:    &bytes.Buffer{},
		Stderr:    &bytes.Buffer{},
	}

	if err := runner.Run(context.Background(), []string{"exec", "postgres", "bash"}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}
	if string(data) != "hello-from-stdin" {
		t.Fatalf("stdin capture = %q, want hello-from-stdin", data)
	}
}

func TestRunnerRunDockerDoesNotPrependCompose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "args.txt")
	scriptPath := filepath.Join(dir, "docker")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$CAPTURE"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}

	runner := Runner{
		DockerBin: scriptPath,
		Env:       []string{"CAPTURE=" + outputPath},
		Stdout:    &bytes.Buffer{},
		Stderr:    &bytes.Buffer{},
	}

	if err := runner.RunDocker(context.Background(), []string{"version", "--format", "json"}, nil); err != nil {
		t.Fatalf("RunDocker() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(args) error = %v", err)
	}
	if string(data) != "version\n--format\njson\n" {
		t.Fatalf("args = %q, want docker args without compose prefix", data)
	}
}

func TestRunComposeLoadsEnvAndFiltersWrapperArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	capturePath := filepath.Join(dir, "capture.json")
	scriptPath := filepath.Join(dir, "docker")

	if err := os.WriteFile(envPath, []byte("TOKEN=from-env-file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	script := `#!/bin/sh
printf '{"args":[' > "$CAPTURE"
first=1
for arg in "$@"; do
  if [ "$first" = 1 ]; then first=0; else printf ',' >> "$CAPTURE"; fi
  printf '"%s"' "$arg" >> "$CAPTURE"
done
printf '],"token":"%s"}' "$TOKEN" >> "$CAPTURE"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}

	t.Setenv("ENVOYAGE_DOCKER_BIN", scriptPath)
	t.Setenv("CAPTURE", capturePath)

	if err := RunCompose(context.Background(), []string{"--env-file", envPath, "-f", "compose.yaml", "config"}); err != nil {
		t.Fatalf("RunCompose() error = %v", err)
	}

	data, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}
	var capture struct {
		Args  []string `json:"args"`
		Token string   `json:"token"`
	}
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatalf("Unmarshal(capture) error = %v; data=%s", err, data)
	}

	wantArgs := []string{"compose", "-f", "compose.yaml", "config"}
	if strings.Join(capture.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", capture.Args, wantArgs)
	}
	if capture.Token != "from-env-file" {
		t.Fatalf("TOKEN = %q, want from-env-file", capture.Token)
	}
}

func TestRunComposeAutomaticallyLoadsDefaultEnvFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	t.Chdir(dir)

	capturePath := filepath.Join(dir, "capture.json")
	scriptPath := filepath.Join(dir, "docker")

	if err := os.WriteFile(".env", []byte("TOKEN=from-default-env\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}
	script := `#!/bin/sh
printf '{"args":[' > "$CAPTURE"
first=1
for arg in "$@"; do
  if [ "$first" = 1 ]; then first=0; else printf ',' >> "$CAPTURE"; fi
  printf '"%s"' "$arg" >> "$CAPTURE"
done
printf '],"token":"%s"}' "$TOKEN" >> "$CAPTURE"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}

	t.Setenv("ENVOYAGE_DOCKER_BIN", scriptPath)
	t.Setenv("CAPTURE", capturePath)

	if err := RunCompose(context.Background(), []string{"-f", "compose.yaml", "config"}); err != nil {
		t.Fatalf("RunCompose() error = %v", err)
	}

	data, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}
	var capture struct {
		Args  []string `json:"args"`
		Token string   `json:"token"`
	}
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatalf("Unmarshal(capture) error = %v; data=%s", err, data)
	}

	wantArgs := []string{"compose", "-f", "compose.yaml", "config"}
	if strings.Join(capture.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", capture.Args, wantArgs)
	}
	if capture.Token != "from-default-env" {
		t.Fatalf("TOKEN = %q, want from-default-env", capture.Token)
	}
}

func TestRunnerDefaultsDockerBinAndDiscardWriters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	capturePath := filepath.Join(dir, "capture.txt")
	dockerPath := filepath.Join(dir, "docker")
	script := `#!/bin/sh
printf '%s:%s' "$1" "$TOKEN" > "$CAPTURE"
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runner := Runner{
		Env: []string{
			"CAPTURE=" + capturePath,
		},
	}

	if err := runner.Run(context.Background(), []string{"config"}, map[string]string{"TOKEN": "from-overlay"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	data, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}
	if string(data) != "compose:from-overlay" {
		t.Fatalf("capture = %q, want compose:from-overlay", data)
	}
}

func TestRunnerReturnsCommandErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	dockerPath := filepath.Join(dir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}

	err := Runner{DockerBin: dockerPath}.Run(context.Background(), []string{"config"}, nil)
	if err == nil {
		t.Fatal("Run() error = nil, want command error")
	}
	if !strings.Contains(err.Error(), "run docker compose") {
		t.Fatalf("error = %q, want run docker compose", err.Error())
	}
}

func TestFindRealDockerBinSkipsShimPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable bit checks are different on Windows")
	}

	dir := t.TempDir()
	shimDir := filepath.Join(dir, "shim")
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(shimDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(shim) error = %v", err)
	}
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(real) error = %v", err)
	}

	shimPath := filepath.Join(shimDir, "docker")
	realPath := filepath.Join(realDir, "docker")
	if err := os.WriteFile(shimPath, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(shim) error = %v", err)
	}
	if err := os.WriteFile(realPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(real) error = %v", err)
	}

	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+realDir)

	got, err := findRealDockerBin(shimPath)
	if err != nil {
		t.Fatalf("findRealDockerBin() error = %v", err)
	}
	if got != realPath {
		t.Fatalf("findRealDockerBin() = %q, want %q", got, realPath)
	}
}

func TestFindRealDockerBinReportsMissingRealDocker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable bit checks are different on Windows")
	}

	dir := t.TempDir()
	shimPath := filepath.Join(dir, "docker")
	if err := os.WriteFile(shimPath, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(shim) error = %v", err)
	}

	t.Setenv("PATH", dir)

	_, err := findRealDockerBin(shimPath)
	if err == nil {
		t.Fatal("findRealDockerBin() error = nil, want missing real docker error")
	}
	if !strings.Contains(err.Error(), "ENVOYAGE_DOCKER_BIN") {
		t.Fatalf("error = %q, want ENVOYAGE_DOCKER_BIN guidance", err.Error())
	}
}

func TestRunComposeReturnsParseAndLoadErrors(t *testing.T) {
	err := RunCompose(context.Background(), []string{"--env-file"})
	if err == nil {
		t.Fatal("RunCompose(parse) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--env-file requires a value") {
		t.Fatalf("parse error = %q, want env-file guidance", err.Error())
	}

	err = RunCompose(context.Background(), []string{"--env-file", filepath.Join(t.TempDir(), "missing.env"), "config"})
	if err == nil {
		t.Fatal("RunCompose(load) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "read env file") {
		t.Fatalf("load error = %q, want read env file", err.Error())
	}
}

func TestNewRunnerDefaultsDockerBin(t *testing.T) {
	t.Setenv("ENVOYAGE_DOCKER_BIN", "")

	runner := NewRunner()
	if runner.DockerBin != "docker" {
		t.Fatalf("DockerBin = %q, want docker", runner.DockerBin)
	}
	if runner.Stdout == nil || runner.Stderr == nil {
		t.Fatal("Stdout/Stderr should default to process streams")
	}
}

func writeEncryptedDotenv(path string, recipient age.Recipient, plaintext string) error {
	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, recipient)
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(plaintext)); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, encrypted.Bytes(), 0o600)
}

func envSliceToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}
