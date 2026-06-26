package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/swoogi/envoyage/internal/ageenv"
	"github.com/swoogi/envoyage/internal/compose"
)

func TestKeygenAndEncryptCommands(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "age-key.txt")
	envPath := filepath.Join(dir, ".env")
	encryptedPath := filepath.Join(dir, ".env.age")

	var keygenOut bytes.Buffer
	if err := runKeygen([]string{"--out", identityPath}, &keygenOut); err != nil {
		t.Fatalf("runKeygen() error = %v", err)
	}
	if !strings.Contains(keygenOut.String(), "recipient: age1") {
		t.Fatalf("keygen output = %q, want recipient", keygenOut.String())
	}
	if strings.Contains(keygenOut.String(), "AGE-SECRET-KEY") {
		t.Fatalf("keygen output leaked private key: %q", keygenOut.String())
	}

	if err := os.WriteFile(envPath, []byte("TOKEN=secret-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}

	var encryptOut bytes.Buffer
	if err := runEncrypt([]string{"--in", envPath, "--out", encryptedPath, "--identity", identityPath}, &encryptOut); err != nil {
		t.Fatalf("runEncrypt() error = %v", err)
	}
	if strings.Contains(encryptOut.String(), "secret-token") {
		t.Fatalf("encrypt output leaked secret: %q", encryptOut.String())
	}

	encrypted, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(encrypted) error = %v", err)
	}
	decrypted, err := ageenv.Decrypt(encrypted, identityPath)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted["TOKEN"] != "secret-token" {
		t.Fatalf("TOKEN = %q, want secret-token", decrypted["TOKEN"])
	}
}

func TestEncryptUsesDefaultPathsAndIdentityEnv(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "age-key.txt")
	envPath := filepath.Join(dir, ".env")
	encryptedPath := filepath.Join(dir, ".env.age")

	if err := runKeygen([]string{"--out", identityPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runKeygen() error = %v", err)
	}
	if err := os.WriteFile(envPath, []byte("TOKEN=secret-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}

	oldInput := defaultEncryptInputPath
	oldOutput := defaultEncryptOutputPath
	defaultEncryptInputPath = envPath
	defaultEncryptOutputPath = encryptedPath
	t.Setenv("AGE_IDENTITY_FILE", identityPath)
	t.Cleanup(func() {
		defaultEncryptInputPath = oldInput
		defaultEncryptOutputPath = oldOutput
	})

	var out bytes.Buffer
	if err := runEncrypt(nil, &out); err != nil {
		t.Fatalf("runEncrypt(defaults) error = %v", err)
	}
	if !strings.Contains(out.String(), "encrypted "+envPath+" -> "+encryptedPath) {
		t.Fatalf("encrypt output = %q, want default paths", out.String())
	}

	encrypted, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(encrypted) error = %v", err)
	}
	decrypted, err := ageenv.Decrypt(encrypted, identityPath)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted["TOKEN"] != "secret-token" {
		t.Fatalf("TOKEN = %q, want secret-token", decrypted["TOKEN"])
	}
}

func TestEncryptUsesDefaultIdentityPathWhenEnvUnset(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "envoyage-key.txt")
	envPath := filepath.Join(dir, ".env")
	encryptedPath := filepath.Join(dir, ".env.age")

	if err := runKeygen([]string{"--out", identityPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runKeygen() error = %v", err)
	}
	if err := os.WriteFile(envPath, []byte("TOKEN=secret-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}

	oldKeygenDefault := defaultKeygenOutputPath
	oldInput := defaultEncryptInputPath
	oldOutput := defaultEncryptOutputPath
	defaultKeygenOutputPath = identityPath
	defaultEncryptInputPath = envPath
	defaultEncryptOutputPath = encryptedPath
	t.Setenv("AGE_IDENTITY_FILE", "")
	t.Cleanup(func() {
		defaultKeygenOutputPath = oldKeygenDefault
		defaultEncryptInputPath = oldInput
		defaultEncryptOutputPath = oldOutput
	})

	if err := runEncrypt(nil, &bytes.Buffer{}); err != nil {
		t.Fatalf("runEncrypt(default identity) error = %v", err)
	}
	encrypted, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(encrypted) error = %v", err)
	}
	decrypted, err := ageenv.Decrypt(encrypted, identityPath)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted["TOKEN"] != "secret-token" {
		t.Fatalf("TOKEN = %q, want secret-token", decrypted["TOKEN"])
	}
}

func TestDecryptCommandUsesDefaultsAndForce(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "age-key.txt")
	secretPath := filepath.Join(dir, ".secrets.env")
	encryptedPath := filepath.Join(dir, ".env.age")
	decryptedPath := filepath.Join(dir, "decrypted.env")

	if err := runKeygen([]string{"--out", identityPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runKeygen() error = %v", err)
	}
	plaintext := []byte("TOKEN=secret-token\n")
	if err := os.WriteFile(secretPath, plaintext, 0o600); err != nil {
		t.Fatalf("WriteFile(secret) error = %v", err)
	}
	if err := runEncrypt([]string{"--in", secretPath, "--out", encryptedPath, "--identity", identityPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runEncrypt() error = %v", err)
	}
	if err := os.WriteFile(decryptedPath, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing decrypted) error = %v", err)
	}

	oldInput := defaultDecryptInputPath
	oldOutput := defaultDecryptOutputPath
	defaultDecryptInputPath = encryptedPath
	defaultDecryptOutputPath = decryptedPath
	t.Setenv("AGE_IDENTITY_FILE", identityPath)
	t.Cleanup(func() {
		defaultDecryptInputPath = oldInput
		defaultDecryptOutputPath = oldOutput
	})

	err := runDecrypt(nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runDecrypt(default overwrite) error = nil, want error")
	}
	existing, err := os.ReadFile(decryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(decrypted existing) error = %v", err)
	}
	if string(existing) != "existing\n" {
		t.Fatalf("decrypted output changed without force: %q", existing)
	}

	var out bytes.Buffer
	if err := runDecrypt([]string{"--force"}, &out); err != nil {
		t.Fatalf("runDecrypt(force) error = %v", err)
	}
	if !strings.Contains(out.String(), "decrypted "+encryptedPath+" -> "+decryptedPath) {
		t.Fatalf("decrypt output = %q, want default paths", out.String())
	}
	decrypted, err := os.ReadFile(decryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(decrypted force) error = %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestKeygenRefusesOverwriteAndEncryptForceFlag(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "age-key.txt")
	envPath := filepath.Join(dir, ".env")
	encryptedPath := filepath.Join(dir, ".env.age")

	var keygenOut bytes.Buffer
	if err := runKeygen([]string{"-o", identityPath}, &keygenOut); err != nil {
		t.Fatalf("runKeygen() error = %v", err)
	}
	if err := runKeygen([]string{"--out", identityPath}, &bytes.Buffer{}); err == nil {
		t.Fatal("runKeygen() overwrite error = nil, want error")
	}

	if err := os.WriteFile(envPath, []byte("TOKEN=secret-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	if err := os.WriteFile(encryptedPath, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(encrypted) error = %v", err)
	}

	err := runEncrypt([]string{"--in", envPath, "-o", encryptedPath, "-i", identityPath}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runEncrypt() overwrite error = nil, want error")
	}
	existing, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(encrypted) error = %v", err)
	}
	if string(existing) != "existing\n" {
		t.Fatalf("encrypted file was overwritten without force: %q", existing)
	}

	if err := runEncrypt([]string{"--in", envPath, "-o", encryptedPath, "-i", identityPath, "--force"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runEncrypt(force) error = %v", err)
	}
}

func TestKeygenUsesDefaultOutputAndForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	defaultDir := filepath.Join(dir, "etc", "envoyage")
	defaultPath := filepath.Join(defaultDir, "envoyage-key.txt")
	oldDefault := defaultKeygenOutputPath
	defaultKeygenOutputPath = defaultPath
	t.Cleanup(func() {
		defaultKeygenOutputPath = oldDefault
	})

	var firstOut bytes.Buffer
	if err := runKeygen(nil, &firstOut); err != nil {
		t.Fatalf("runKeygen(default) error = %v", err)
	}
	if !strings.Contains(firstOut.String(), "identity: "+defaultPath) {
		t.Fatalf("keygen output = %q, want default identity path", firstOut.String())
	}

	dirInfo, err := os.Stat(defaultDir)
	if err != nil {
		t.Fatalf("Stat(default dir) error = %v", err)
	}
	if dirInfo.Mode().Perm() != 0o750 {
		t.Fatalf("default dir permissions = %v, want 0750", dirInfo.Mode().Perm())
	}

	firstData, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatalf("ReadFile(default identity) error = %v", err)
	}
	if !strings.Contains(string(firstData), "AGE-SECRET-KEY") {
		t.Fatalf("default identity does not look like an age identity: %q", firstData)
	}
	fileInfo, err := os.Stat(defaultPath)
	if err != nil {
		t.Fatalf("Stat(default identity) error = %v", err)
	}
	if fileInfo.Mode().Perm() != 0o640 {
		t.Fatalf("default identity permissions = %v, want 0640", fileInfo.Mode().Perm())
	}

	if err := runKeygen(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("runKeygen(default overwrite) error = nil, want error")
	}
	unchanged, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatalf("ReadFile(default identity after overwrite attempt) error = %v", err)
	}
	if !bytes.Equal(unchanged, firstData) {
		t.Fatal("default identity changed without --force")
	}

	if err := runKeygen([]string{"--force"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runKeygen(default force) error = %v", err)
	}
	overwritten, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatalf("ReadFile(default identity after force) error = %v", err)
	}
	if bytes.Equal(overwritten, firstData) {
		t.Fatal("default identity was not overwritten with --force")
	}
}

func TestKeygenSystemDefaultRequiresRoot(t *testing.T) {
	if getEUID() == 0 {
		t.Skip("root can create the system default identity path")
	}

	oldDefault := defaultKeygenOutputPath
	defaultKeygenOutputPath = compose.DefaultIdentityFile
	t.Cleanup(func() {
		defaultKeygenOutputPath = oldDefault
	})

	err := runKeygen(nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runKeygen(system default) error = nil, want root guidance")
	}
	if !strings.Contains(err.Error(), "requires root") {
		t.Fatalf("error = %q, want requires root", err.Error())
	}
}

func TestDefaultKeygenPermissionsUseDockerGroup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "etc", "envoyage", "envoyage-key.txt")
	restoreKeygenPermissionHooks(t)

	systemIdentityFile = path
	getEUID = func() int { return 0 }
	lookupDockerGroupID = func() (int, bool) { return 4242, true }
	var chownCalls []string
	chown = func(path string, uid int, gid int) error {
		chownCalls = append(chownCalls, path)
		if uid != 0 || gid != 4242 {
			t.Fatalf("chown(%q) = uid:%d gid:%d, want uid:0 gid:4242", path, uid, gid)
		}
		return nil
	}

	if err := prepareDefaultKeygenOutput(path); err != nil {
		t.Fatalf("prepareDefaultKeygenOutput() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("AGE-SECRET-KEY-1TEST\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(identity) error = %v", err)
	}
	if err := applyDefaultKeygenPermissions(path); err != nil {
		t.Fatalf("applyDefaultKeygenPermissions() error = %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat(default dir) error = %v", err)
	}
	if dirInfo.Mode().Perm() != 0o750 {
		t.Fatalf("default dir permissions = %v, want 0750", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(identity) error = %v", err)
	}
	if fileInfo.Mode().Perm() != 0o640 {
		t.Fatalf("identity permissions = %v, want 0640", fileInfo.Mode().Perm())
	}
	if len(chownCalls) != 2 {
		t.Fatalf("chown calls = %#v, want dir and file", chownCalls)
	}
}

func TestDefaultKeygenPermissionsFallbackRootOnlyWithoutDockerGroup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "etc", "envoyage", "envoyage-key.txt")
	restoreKeygenPermissionHooks(t)

	systemIdentityFile = path
	getEUID = func() int { return 0 }
	lookupDockerGroupID = func() (int, bool) { return 0, false }
	chown = func(path string, uid int, gid int) error {
		if uid != 0 || gid != 0 {
			t.Fatalf("chown(%q) = uid:%d gid:%d, want root:root", path, uid, gid)
		}
		return nil
	}

	if err := prepareDefaultKeygenOutput(path); err != nil {
		t.Fatalf("prepareDefaultKeygenOutput() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("AGE-SECRET-KEY-1TEST\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(identity) error = %v", err)
	}
	if err := applyDefaultKeygenPermissions(path); err != nil {
		t.Fatalf("applyDefaultKeygenPermissions() error = %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat(default dir) error = %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("default dir permissions = %v, want 0700", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(identity) error = %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("identity permissions = %v, want 0600", fileInfo.Mode().Perm())
	}
}

func TestDefaultKeygenPermissionErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "etc", "envoyage", "envoyage-key.txt")
	restoreKeygenPermissionHooks(t)

	chmod = func(path string, mode os.FileMode) error {
		return os.ErrPermission
	}
	err := prepareDefaultKeygenOutput(path)
	if err == nil {
		t.Fatal("prepareDefaultKeygenOutput() error = nil, want chmod error")
	}
	if !strings.Contains(err.Error(), "set default identity directory permissions") {
		t.Fatalf("prepare error = %q, want directory permissions", err.Error())
	}

	restoreKeygenPermissionHooks(t)
	chmod = func(path string, mode os.FileMode) error {
		return os.ErrPermission
	}
	err = applyDefaultKeygenPermissions(path)
	if err == nil {
		t.Fatal("applyDefaultKeygenPermissions() error = nil, want chmod error")
	}
	if !strings.Contains(err.Error(), "set default identity file permissions") {
		t.Fatalf("apply error = %q, want file permissions", err.Error())
	}
}

func TestDockerGroupID(t *testing.T) {
	gid, ok := dockerGroupID()
	if !ok {
		t.Skip("docker group is not available on this host")
	}
	if gid <= 0 {
		t.Fatalf("dockerGroupID() = %d, want positive gid", gid)
	}
}

func TestEncryptCommandWithRecipientAlias(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "age-key.txt")
	envPath := filepath.Join(dir, ".env")
	encryptedPath := filepath.Join(dir, ".env.age")

	var keygenOut bytes.Buffer
	if err := runKeygen([]string{"--out", identityPath}, &keygenOut); err != nil {
		t.Fatalf("runKeygen() error = %v", err)
	}
	var recipient string
	for _, line := range strings.Split(keygenOut.String(), "\n") {
		if strings.HasPrefix(line, "recipient: ") {
			recipient = strings.TrimPrefix(line, "recipient: ")
		}
	}
	if recipient == "" {
		t.Fatalf("recipient not found in output: %q", keygenOut.String())
	}
	if err := os.WriteFile(envPath, []byte("TOKEN=secret-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}

	if err := runEncrypt([]string{"--in", envPath, "--out", encryptedPath, "-r", recipient}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runEncrypt() error = %v", err)
	}
	encrypted, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(encrypted) error = %v", err)
	}
	decrypted, err := ageenv.Decrypt(encrypted, identityPath)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted["TOKEN"] != "secret-token" {
		t.Fatalf("TOKEN = %q, want secret-token", decrypted["TOKEN"])
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"unknown"})
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `unknown command "unknown"`) {
		t.Fatalf("error = %q, want unknown command", err.Error())
	}
}

func TestRunForProgramDockerShimInterceptsCompose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	capturePath := filepath.Join(dir, "capture.json")
	dockerPath := filepath.Join(dir, "real-docker")

	if err := os.WriteFile(envPath, []byte("TOKEN=from-shim-env\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	if err := os.WriteFile(dockerPath, []byte(captureDockerScript), 0o700); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}

	t.Setenv("ENVOYAGE_DOCKER_BIN", dockerPath)
	t.Setenv("CAPTURE", capturePath)

	err := runForProgram("docker", []string{"compose", "--env-file", envPath, "-f", "compose.yaml", "config"})
	if err != nil {
		t.Fatalf("runForProgram(docker compose) error = %v", err)
	}

	capture := readDockerCapture(t, capturePath)
	wantArgs := []string{"compose", "-f", "compose.yaml", "config"}
	if strings.Join(capture.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", capture.Args, wantArgs)
	}
	if capture.Token != "from-shim-env" {
		t.Fatalf("TOKEN = %q, want from-shim-env", capture.Token)
	}
}

func TestRunForProgramDockerShimPassesThroughNonCompose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	capturePath := filepath.Join(dir, "capture.json")
	dockerPath := filepath.Join(dir, "real-docker")

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=should-not-load\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	if err := os.WriteFile(dockerPath, []byte(captureDockerScript), 0o700); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}

	t.Chdir(dir)
	t.Setenv("ENVOYAGE_DOCKER_BIN", dockerPath)
	t.Setenv("CAPTURE", capturePath)
	t.Setenv("TOKEN", "")

	err := runForProgram("docker", []string{"version", "--format", "{{.Server.Version}}"})
	if err != nil {
		t.Fatalf("runForProgram(docker version) error = %v", err)
	}

	capture := readDockerCapture(t, capturePath)
	wantArgs := []string{"version", "--format", "{{.Server.Version}}"}
	if strings.Join(capture.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", capture.Args, wantArgs)
	}
	if capture.Token != "" {
		t.Fatalf("TOKEN = %q, want no default env loading", capture.Token)
	}
}

func TestRunForProgramPodmanShimInterceptsCompose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	capturePath := filepath.Join(dir, "capture.json")
	podmanPath := filepath.Join(dir, "real-podman")

	if err := os.WriteFile(envPath, []byte("TOKEN=from-podman-shim\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	if err := os.WriteFile(podmanPath, []byte(captureDockerScript), 0o700); err != nil {
		t.Fatalf("WriteFile(podman) error = %v", err)
	}

	t.Setenv("ENVOYAGE_PODMAN_BIN", podmanPath)
	t.Setenv("CAPTURE", capturePath)

	err := runForProgram("podman", []string{"compose", "--env-file", envPath, "config"})
	if err != nil {
		t.Fatalf("runForProgram(podman compose) error = %v", err)
	}

	capture := readDockerCapture(t, capturePath)
	wantArgs := []string{"compose", "config"}
	if strings.Join(capture.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", capture.Args, wantArgs)
	}
	if capture.Token != "from-podman-shim" {
		t.Fatalf("TOKEN = %q, want podman shim env", capture.Token)
	}
}

func TestRunForProgramPodmanShimPassesThroughNonCompose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is POSIX-specific")
	}

	dir := t.TempDir()
	capturePath := filepath.Join(dir, "capture.json")
	podmanPath := filepath.Join(dir, "real-podman")

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=should-not-load\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	if err := os.WriteFile(podmanPath, []byte(captureDockerScript), 0o700); err != nil {
		t.Fatalf("WriteFile(podman) error = %v", err)
	}

	t.Chdir(dir)
	t.Setenv("ENVOYAGE_PODMAN_BIN", podmanPath)
	t.Setenv("CAPTURE", capturePath)
	t.Setenv("TOKEN", "")

	err := runForProgram("podman", []string{"ps", "--all"})
	if err != nil {
		t.Fatalf("runForProgram(podman ps) error = %v", err)
	}

	capture := readDockerCapture(t, capturePath)
	wantArgs := []string{"ps", "--all"}
	if strings.Join(capture.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", capture.Args, wantArgs)
	}
	if capture.Token != "" {
		t.Fatalf("TOKEN = %q, want no default env loading", capture.Token)
	}
}

func TestDockerShimNameIncludesWindowsExecutable(t *testing.T) {
	if !isDockerShimName("docker") {
		t.Fatal("docker should be treated as a shim name")
	}
	if !isDockerShimName("docker.exe") {
		t.Fatal("docker.exe should be treated as a shim name")
	}
	if isDockerShimName("envoyage") {
		t.Fatal("envoyage should not be treated as a docker shim name")
	}
	if runtimeName, ok := shimRuntimeName("podman"); !ok || runtimeName != "podman" {
		t.Fatalf("shimRuntimeName(podman) = %q, %v, want podman true", runtimeName, ok)
	}
	if runtimeName, ok := shimRuntimeName("podman.exe"); !ok || runtimeName != "podman" {
		t.Fatalf("shimRuntimeName(podman.exe) = %q, %v, want podman true", runtimeName, ok)
	}
}

func TestInstallStatusAndUninstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source-envoyage")
	binDir := filepath.Join(dir, "bin")
	libDir := filepath.Join(dir, "lib", "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.WriteFile(source, []byte("envoyage-binary\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}

	var installOut bytes.Buffer
	if err := runInstall([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &installOut); err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}
	target := filepath.Join(libDir, "envoyage")
	link := filepath.Join(binDir, "envoyage")
	assertFileContent(t, target, "envoyage-binary\n")
	assertSymlinkTarget(t, link, target)
	assertContains(t, installOut.String(), "installed envoyage")

	var statusOut bytes.Buffer
	if err := runStatus([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &statusOut); err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}
	assertContains(t, statusOut.String(), "installed binary: yes")
	assertContains(t, statusOut.String(), "command symlink installed: yes")

	var uninstallOut bytes.Buffer
	if err := runUninstall([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &uninstallOut); err != nil {
		t.Fatalf("runUninstall() error = %v", err)
	}
	assertContains(t, uninstallOut.String(), "removed command symlink")
	assertContains(t, uninstallOut.String(), "hash -r")
	assertPathsRemoved(t, link, target)
}

func TestInstallUsesDefaultHomeDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source-envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.WriteFile(source, []byte("envoyage-binary\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := runInstall(nil, &bytes.Buffer{}); err != nil {
		t.Fatalf("runInstall(defaults) error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".local", "lib", "envoyage", "envoyage")); err != nil {
		t.Fatalf("default install target missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".local", "bin", "envoyage")); err != nil {
		t.Fatalf("default command symlink missing: %v", err)
	}
}

func TestInstallUsesSystemDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source-envoyage")
	systemBinDir := filepath.Join(dir, "usr", "local", "bin")
	systemLibDir := filepath.Join(dir, "usr", "local", "lib", "envoyage")
	restoreShimHooks(t)
	restoreSystemInstallDefaults(t, systemBinDir, systemLibDir)
	osExecutable = func() (string, error) { return source, nil }

	if err := os.WriteFile(source, []byte("envoyage-binary\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := runInstall([]string{"--system"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runInstall(system) error = %v", err)
	}

	target := filepath.Join(systemLibDir, "envoyage")
	link := filepath.Join(systemBinDir, "envoyage")
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("system install target missing: %v", err)
	}
	linkTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink(system command) error = %v", err)
	}
	if linkTarget != target {
		t.Fatalf("system command target = %q, want %q", linkTarget, target)
	}
}

func TestUninstallWithoutModeRemovesUserAndSystemInstalls(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source-envoyage")
	userTarget := filepath.Join(dir, ".local", "lib", "envoyage", "envoyage")
	userLink := filepath.Join(dir, ".local", "bin", "envoyage")
	userShim := filepath.Join(dir, ".local", "bin", "docker")
	systemBinDir := filepath.Join(dir, "usr", "local", "bin")
	systemLibDir := filepath.Join(dir, "usr", "local", "lib", "envoyage")
	systemTarget := filepath.Join(systemLibDir, "envoyage")
	systemLink := filepath.Join(systemBinDir, "envoyage")
	systemShim := filepath.Join(systemBinDir, "docker")
	restoreShimHooks(t)
	restoreSystemInstallDefaults(t, systemBinDir, systemLibDir)
	restoreSystemShimDefault(t, systemBinDir)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.WriteFile(source, []byte("envoyage-binary\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := runInstall(nil, &bytes.Buffer{}); err != nil {
		t.Fatalf("runInstall(user) error = %v", err)
	}
	if err := runInstall([]string{"--system"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runInstall(system) error = %v", err)
	}
	if err := runShim([]string{"install", "--runtime", "docker"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(user install) error = %v", err)
	}
	if err := runShim([]string{"install", "--runtime", "docker", "--system"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(system install) error = %v", err)
	}

	var out bytes.Buffer
	if err := runUninstall(nil, &out); err != nil {
		t.Fatalf("runUninstall(auto) error = %v", err)
	}
	assertPathsRemoved(t, userTarget, userLink, userShim, systemTarget, systemLink, systemShim)
	assertOutputOrder(t, out.String(), "removed shim: "+userShim, "removed command symlink: "+userLink)
	assertOutputOrder(t, out.String(), "removed shim: "+systemShim, "removed command symlink: "+systemLink)
}

func TestUninstallSkipsNonEnvoyageDocker(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source-envoyage")
	binDir := filepath.Join(dir, "bin")
	libDir := filepath.Join(dir, "lib")
	target := filepath.Join(libDir, "envoyage")
	link := filepath.Join(binDir, "envoyage")
	dockerPath := filepath.Join(binDir, "docker")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }

	if err := os.WriteFile(source, []byte("envoyage-binary\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := runInstall([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}
	if err := os.WriteFile(dockerPath, []byte("real-docker\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}

	var out bytes.Buffer
	if err := runUninstall([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &out); err != nil {
		t.Fatalf("runUninstall(non-envoyage docker) error = %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("envoyage target still exists after uninstall: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("envoyage symlink still exists after uninstall: %v", err)
	}
	if _, err := os.Lstat(dockerPath); err != nil {
		t.Fatalf("non-envoyage docker should remain: %v", err)
	}
	if !strings.Contains(out.String(), "shim not managed by Envoyage") {
		t.Fatalf("uninstall output = %q, want non-managed docker note", out.String())
	}
}

func TestInstallSystemRejectsCustomDirectories(t *testing.T) {
	dir := t.TempDir()

	err := runInstall([]string{"--system", "--bin-dir", filepath.Join(dir, "bin")}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runInstall(system custom bin) error = nil, want conflict")
	}
	if !strings.Contains(err.Error(), "--system cannot be combined") {
		t.Fatalf("install error = %q, want system conflict", err.Error())
	}

	err = runStatus([]string{"--system", "--lib-dir", filepath.Join(dir, "lib")}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runStatus(system custom lib) error = nil, want conflict")
	}
	if !strings.Contains(err.Error(), "--system cannot be combined") {
		t.Fatalf("status error = %q, want system conflict", err.Error())
	}
}

func TestInstallForceReplacesInstalledBinaryAndSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source-envoyage")
	binDir := filepath.Join(dir, "bin")
	libDir := filepath.Join(dir, "lib")
	target := filepath.Join(libDir, "envoyage")
	link := filepath.Join(binDir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.MkdirAll(libDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(lib) error = %v", err)
	}
	if err := os.WriteFile(source, []byte("new-binary\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := os.WriteFile(target, []byte("old-binary\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink(link) error = %v", err)
	}

	if err := runInstall([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &bytes.Buffer{}); err == nil {
		t.Fatal("runInstall(existing target) error = nil, want overwrite guidance")
	}
	if err := runInstall([]string{"--bin-dir", binDir, "--lib-dir", libDir, "--force"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runInstall(force) error = %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if string(data) != "new-binary\n" {
		t.Fatalf("target after force = %q, want new binary", data)
	}

	osExecutable = func() (string, error) { return target, nil }
	var out bytes.Buffer
	if err := runInstall([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &out); err != nil {
		t.Fatalf("runInstall(already installed) error = %v", err)
	}
	if !strings.Contains(out.String(), "command symlink already installed") {
		t.Fatalf("install output = %q, want already installed symlink", out.String())
	}
}

func TestInstallRefusesNonEnvoyageCommand(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source-envoyage")
	binDir := filepath.Join(dir, "bin")
	libDir := filepath.Join(dir, "lib")
	link := filepath.Join(binDir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.WriteFile(source, []byte("envoyage-binary\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := os.WriteFile(link, []byte("other-command\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(link) error = %v", err)
	}

	err := runInstall([]string{"--bin-dir", binDir, "--lib-dir", libDir, "--force"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runInstall(non-envoyage command) error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite non-Envoyage command") {
		t.Fatalf("install error = %q, want overwrite refusal", err.Error())
	}
}

func TestUninstallRefusesNonEnvoyageCommand(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	libDir := filepath.Join(dir, "lib")
	link := filepath.Join(binDir, "envoyage")

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.WriteFile(link, []byte("other-command\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(link) error = %v", err)
	}

	err := runUninstall([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runUninstall(non-envoyage command) error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "refusing to remove non-Envoyage command") {
		t.Fatalf("uninstall error = %q, want remove refusal", err.Error())
	}
}

func TestUninstallMissingIsNoop(t *testing.T) {
	dir := t.TempDir()

	var out bytes.Buffer
	if err := runUninstall([]string{"--bin-dir", filepath.Join(dir, "bin"), "--lib-dir", filepath.Join(dir, "lib")}, &out); err != nil {
		t.Fatalf("runUninstall(missing) error = %v", err)
	}
	if !strings.Contains(out.String(), "command symlink not installed") {
		t.Fatalf("uninstall output = %q, want symlink missing", out.String())
	}
	if !strings.Contains(out.String(), "installed binary not found") {
		t.Fatalf("uninstall output = %q, want binary missing", out.String())
	}
}

func TestInstallRejectsDirectoryTargets(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source-envoyage")
	binDir := filepath.Join(dir, "bin")
	libDir := filepath.Join(dir, "lib")
	target := filepath.Join(libDir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }

	if err := os.WriteFile(source, []byte("envoyage-binary\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("MkdirAll(target dir) error = %v", err)
	}

	err := runInstall([]string{"--bin-dir", binDir, "--lib-dir", libDir, "--force"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runInstall(directory target) error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite directory") {
		t.Fatalf("install error = %q, want directory refusal", err.Error())
	}

	err = runUninstall([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runUninstall(directory target) error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "refusing to remove directory") {
		t.Fatalf("uninstall error = %q, want directory refusal", err.Error())
	}
}

func TestInstallReportsSourceExecutableErrors(t *testing.T) {
	dir := t.TempDir()
	missingSource := filepath.Join(dir, "missing-envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return missingSource, nil }

	err := runInstall([]string{"--bin-dir", filepath.Join(dir, "bin"), "--lib-dir", filepath.Join(dir, "lib")}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runInstall(missing source) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "open source executable") {
		t.Fatalf("install error = %q, want source executable error", err.Error())
	}
}

func TestInstallStatusDispatchAndValidation(t *testing.T) {
	output := captureStdout(t, func() {
		if err := run([]string{"status", "--bin-dir", filepath.Join(t.TempDir(), "bin"), "--lib-dir", filepath.Join(t.TempDir(), "lib")}); err != nil {
			t.Fatalf("run(status) error = %v", err)
		}
	})
	if !strings.Contains(output, "install target:") {
		t.Fatalf("status output = %q, want install target", output)
	}

	tests := [][]string{
		{"install", "extra"},
		{"uninstall", "extra"},
		{"status", "extra"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			err := run(args)
			if err == nil {
				t.Fatal("run() error = nil, want extra args error")
			}
			if !strings.Contains(err.Error(), "does not accept arguments") {
				t.Fatalf("error = %q, want extra args error", err.Error())
			}
		})
	}
}

func TestInstallRejectsEmptyDirectories(t *testing.T) {
	err := runInstall([]string{"--bin-dir", ""}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runInstall(empty bin dir) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--bin-dir is required") {
		t.Fatalf("install error = %q, want bin-dir guidance", err.Error())
	}

	err = runStatus([]string{"--lib-dir", ""}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runStatus(empty lib dir) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--lib-dir is required") {
		t.Fatalf("status error = %q, want directory guidance", err.Error())
	}
}

func TestShimInstallStatusAndUninstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	source := filepath.Join(dir, "envoyage")
	installTarget := filepath.Join(dir, ".local", "lib", "envoyage", "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	var installOut bytes.Buffer
	if err := runShim([]string{"install", "--runtime", "docker", "--bin-dir", binDir}, &installOut); err != nil {
		t.Fatalf("runShim(install) error = %v", err)
	}
	shimPath := filepath.Join(binDir, "docker")
	assertSymlinkTarget(t, shimPath, installTarget)
	assertPathExists(t, installTarget)
	assertContains(t, installOut.String(), "installed docker shim")

	var statusOut bytes.Buffer
	if err := runShim([]string{"status", "--runtime", "docker", "--bin-dir", binDir}, &statusOut); err != nil {
		t.Fatalf("runShim(status) error = %v", err)
	}
	assertContains(t, statusOut.String(), "installed: yes")
	assertContains(t, statusOut.String(), "shim target: "+installTarget)

	var secondInstallOut bytes.Buffer
	if err := runShim([]string{"install", "--runtime", "docker", "--bin-dir", binDir}, &secondInstallOut); err != nil {
		t.Fatalf("runShim(install existing) error = %v", err)
	}
	assertContains(t, secondInstallOut.String(), "shim already installed")

	var uninstallOut bytes.Buffer
	if err := runShim([]string{"uninstall", "--runtime", "docker", "--bin-dir", binDir}, &uninstallOut); err != nil {
		t.Fatalf("runShim(uninstall) error = %v", err)
	}
	assertContains(t, uninstallOut.String(), "removed shim")
	assertContains(t, uninstallOut.String(), "hash -r")
	assertPathsRemoved(t, shimPath)
}

func TestShimInstallAutoDetectsDockerAndPodman(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	realDir := filepath.Join(dir, "real")
	source := filepath.Join(dir, "envoyage")
	installTarget := filepath.Join(dir, ".local", "lib", "envoyage", "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(real) error = %v", err)
	}
	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	for _, name := range []string{"docker", "podman"} {
		if err := os.WriteFile(filepath.Join(realDir, name), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}

	t.Setenv("PATH", realDir)
	var out bytes.Buffer
	if err := runShim([]string{"install", "--bin-dir", binDir}, &out); err != nil {
		t.Fatalf("runShim(auto install) error = %v", err)
	}

	assertSymlinkTarget(t, filepath.Join(binDir, "docker"), installTarget)
	assertSymlinkTarget(t, filepath.Join(binDir, "podman"), installTarget)
	assertContains(t, out.String(), "installed docker shim")
	assertContains(t, out.String(), "installed podman shim")
}

func TestShimInstallAllCreatesDockerAndPodmanWithoutDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	source := filepath.Join(dir, "envoyage")
	installTarget := filepath.Join(dir, ".local", "lib", "envoyage", "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }
	t.Setenv("PATH", filepath.Join(dir, "empty"))

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}

	var out bytes.Buffer
	if err := runShim([]string{"install", "--runtime", "all", "--bin-dir", binDir}, &out); err != nil {
		t.Fatalf("runShim(all install) error = %v", err)
	}

	assertSymlinkTarget(t, filepath.Join(binDir, "docker"), installTarget)
	assertSymlinkTarget(t, filepath.Join(binDir, "podman"), installTarget)
	assertContains(t, out.String(), "installed docker shim")
	assertContains(t, out.String(), "installed podman shim")
}

func TestShimInstallAutoUsesRuntimeEnvOverrides(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	source := filepath.Join(dir, "envoyage")
	installTarget := filepath.Join(dir, ".local", "lib", "envoyage", "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }
	t.Setenv("PATH", filepath.Join(dir, "empty"))
	t.Setenv("ENVOYAGE_DOCKER_BIN", "")
	t.Setenv("ENVOYAGE_PODMAN_BIN", "/custom/podman")

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}

	var out bytes.Buffer
	if err := runShim([]string{"install", "--bin-dir", binDir}, &out); err != nil {
		t.Fatalf("runShim(auto install env override) error = %v", err)
	}

	assertPathExists(t, filepath.Join(binDir, "podman"))
	if _, err := os.Lstat(filepath.Join(binDir, "docker")); !os.IsNotExist(err) {
		t.Fatalf("docker shim should not be installed when only podman is detected: %v", err)
	}
	assertSymlinkTarget(t, filepath.Join(binDir, "podman"), installTarget)
	assertContains(t, out.String(), "installed podman shim")
}

func TestShimInstallAutoRequiresDetectedRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable bit checks are different on Windows")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }
	t.Setenv("PATH", filepath.Join(dir, "empty"))
	t.Setenv("ENVOYAGE_DOCKER_BIN", "")
	t.Setenv("ENVOYAGE_PODMAN_BIN", "")

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}

	err := runShim([]string{"install", "--bin-dir", filepath.Join(dir, "bin")}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runShim(auto install without runtime) error = nil, want error")
	}
	assertContains(t, err.Error(), "no supported runtime found")
}

func TestShimRejectsUnsupportedRuntime(t *testing.T) {
	tests := [][]string{
		{"install", "--runtime", "nerdctl"},
		{"status", "--runtime", "nerdctl"},
		{"uninstall", "--runtime", "nerdctl"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			err := runShim(args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("runShim() error = nil, want unsupported runtime error")
			}
			assertContains(t, err.Error(), `unsupported shim runtime "nerdctl"`)
		})
	}
}

func TestShimInstallForceRecreatesExistingShim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	source := filepath.Join(dir, "envoyage")
	installTarget := filepath.Join(dir, ".local", "lib", "envoyage", "envoyage")
	shimPath := filepath.Join(binDir, "docker")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	if err := os.Symlink(source, shimPath); err != nil {
		t.Fatalf("Symlink(existing shim) error = %v", err)
	}

	if err := runShim([]string{"install", "--runtime", "docker", "--bin-dir", binDir, "--force"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(install force) error = %v", err)
	}
	linkTarget, err := os.Readlink(shimPath)
	if err != nil {
		t.Fatalf("Readlink(shim) error = %v", err)
	}
	if linkTarget != installTarget {
		t.Fatalf("shim target = %q, want %q", linkTarget, installTarget)
	}
}

func TestShimUninstallMissingIsNoop(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }

	var out bytes.Buffer
	if err := runShim([]string{"uninstall", "--runtime", "docker", "--bin-dir", filepath.Join(dir, "bin")}, &out); err != nil {
		t.Fatalf("runShim(uninstall missing) error = %v", err)
	}
	if !strings.Contains(out.String(), "shim not installed") {
		t.Fatalf("uninstall missing output = %q, want not installed", out.String())
	}
}

func TestShimRefusesNonEnvoyageDocker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	source := filepath.Join(dir, "envoyage")
	shimPath := filepath.Join(binDir, "docker")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	if err := os.WriteFile(shimPath, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(existing docker) error = %v", err)
	}

	err := runShim([]string{"install", "--runtime", "docker", "--bin-dir", binDir, "--force"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runShim(install non-envoyage) error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite non-Envoyage docker") {
		t.Fatalf("install error = %q, want overwrite refusal", err.Error())
	}

	err = runShim([]string{"uninstall", "--runtime", "docker", "--bin-dir", binDir}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runShim(uninstall non-envoyage) error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "refusing to remove non-Envoyage docker") {
		t.Fatalf("uninstall error = %q, want remove refusal", err.Error())
	}
}

func TestShimRefusesNonEnvoyagePodman(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	source := filepath.Join(dir, "envoyage")
	shimPath := filepath.Join(binDir, "podman")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	if err := os.WriteFile(shimPath, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(existing podman) error = %v", err)
	}

	err := runShim([]string{"install", "--runtime", "podman", "--bin-dir", binDir, "--force"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runShim(install non-envoyage podman) error = nil, want refusal")
	}
	assertContains(t, err.Error(), "refusing to overwrite non-Envoyage podman")

	err = runShim([]string{"uninstall", "--runtime", "podman", "--bin-dir", binDir}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runShim(uninstall non-envoyage podman) error = nil, want refusal")
	}
	assertContains(t, err.Error(), "refusing to remove non-Envoyage podman")
}

func TestShimStatusWithDockerBinEnv(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	t.Setenv("ENVOYAGE_DOCKER_BIN", "/custom/docker")

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	var out bytes.Buffer
	if err := runShim([]string{"status", "--runtime", "docker", "--bin-dir", filepath.Join(dir, "bin")}, &out); err != nil {
		t.Fatalf("runShim(status) error = %v", err)
	}
	if !strings.Contains(out.String(), "ENVOYAGE_DOCKER_BIN: /custom/docker") {
		t.Fatalf("status output = %q, want configured docker bin", out.String())
	}
}

func TestShimStatusAutoReportsDockerAndPodman(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	t.Setenv("ENVOYAGE_DOCKER_BIN", "/custom/docker")
	t.Setenv("ENVOYAGE_PODMAN_BIN", "/custom/podman")

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	var out bytes.Buffer
	if err := runShim([]string{"status", "--bin-dir", filepath.Join(dir, "bin")}, &out); err != nil {
		t.Fatalf("runShim(status auto) error = %v", err)
	}
	assertContains(t, out.String(), "runtime: docker")
	assertContains(t, out.String(), "runtime: podman")
	assertContains(t, out.String(), "ENVOYAGE_DOCKER_BIN: /custom/docker")
	assertContains(t, out.String(), "ENVOYAGE_PODMAN_BIN: /custom/podman")
}

func TestShimUsesDefaultHomeBinDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "envoyage")
	installTarget := filepath.Join(dir, ".local", "lib", "envoyage", "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	if err := runShim([]string{"install", "--runtime", "docker"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(default install) error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".local", "bin", "docker")); err != nil {
		t.Fatalf("default shim was not installed: %v", err)
	}
	linkTarget, err := os.Readlink(filepath.Join(dir, ".local", "bin", "docker"))
	if err != nil {
		t.Fatalf("Readlink(default shim) error = %v", err)
	}
	if linkTarget != installTarget {
		t.Fatalf("default shim target = %q, want %q", linkTarget, installTarget)
	}
}

func TestShimUsesSystemBinDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "envoyage")
	systemBinDir := filepath.Join(dir, "usr", "local", "bin")
	systemLibDir := filepath.Join(dir, "usr", "local", "lib", "envoyage")
	installTarget := filepath.Join(systemLibDir, "envoyage")
	restoreShimHooks(t)
	restoreSystemInstallDefaults(t, systemBinDir, systemLibDir)
	restoreSystemShimDefault(t, systemBinDir)
	osExecutable = func() (string, error) { return source, nil }

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	if err := runShim([]string{"install", "--runtime", "docker", "--system"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(system install) error = %v", err)
	}
	linkTarget, err := os.Readlink(filepath.Join(systemBinDir, "docker"))
	if err != nil {
		t.Fatalf("Readlink(system shim) error = %v", err)
	}
	if linkTarget != installTarget {
		t.Fatalf("system shim target = %q, want %q", linkTarget, installTarget)
	}
}

func TestShimUninstallWithoutModeRemovesUserAndSystemShims(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "envoyage")
	userShim := filepath.Join(dir, ".local", "bin", "docker")
	systemBinDir := filepath.Join(dir, "usr", "local", "bin")
	systemLibDir := filepath.Join(dir, "usr", "local", "lib", "envoyage")
	systemShim := filepath.Join(systemBinDir, "docker")
	restoreShimHooks(t)
	restoreSystemInstallDefaults(t, systemBinDir, systemLibDir)
	restoreSystemShimDefault(t, systemBinDir)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	if err := runShim([]string{"install", "--runtime", "docker"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(user install) error = %v", err)
	}
	if err := runShim([]string{"install", "--runtime", "docker", "--system"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(system install) error = %v", err)
	}

	var out bytes.Buffer
	if err := runShim([]string{"uninstall"}, &out); err != nil {
		t.Fatalf("runShim(auto uninstall) error = %v", err)
	}
	for _, path := range []string{userShim, systemShim} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after automatic shim uninstall: %v", path, err)
		}
	}
	if !strings.Contains(out.String(), "removed shim: "+userShim) {
		t.Fatalf("shim uninstall output = %q, want user shim removal", out.String())
	}
	if !strings.Contains(out.String(), "removed shim: "+systemShim) {
		t.Fatalf("shim uninstall output = %q, want system shim removal", out.String())
	}
}

func TestShimUninstallAllRemovesDockerAndPodman(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	source := filepath.Join(dir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return source, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.WriteFile(source, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := runShim([]string{"install", "--runtime", "all", "--bin-dir", binDir}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(all install) error = %v", err)
	}

	var out bytes.Buffer
	if err := runShim([]string{"uninstall", "--runtime", "all", "--bin-dir", binDir}, &out); err != nil {
		t.Fatalf("runShim(all uninstall) error = %v", err)
	}

	assertPathsRemoved(t, filepath.Join(binDir, "docker"), filepath.Join(binDir, "podman"))
	assertContains(t, out.String(), "removed shim: "+filepath.Join(binDir, "docker"))
	assertContains(t, out.String(), "removed shim: "+filepath.Join(binDir, "podman"))
}

func TestShimSystemRejectsCustomBinDir(t *testing.T) {
	dir := t.TempDir()

	tests := [][]string{
		{"install", "--runtime", "docker", "--system", "--bin-dir", filepath.Join(dir, "bin")},
		{"status", "--runtime", "docker", "--system", "--bin-dir", filepath.Join(dir, "bin")},
		{"uninstall", "--runtime", "docker", "--system", "--bin-dir", filepath.Join(dir, "bin")},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			err := runShim(args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("runShim(system custom bin) error = nil, want conflict")
			}
			assertContains(t, err.Error(), "--system cannot be combined")
		})
	}
}

func TestShimHelpAndRunDispatch(t *testing.T) {
	output := captureStdout(t, func() {
		if err := run([]string{"shim", "--help"}); err != nil {
			t.Fatalf("run(shim help) error = %v", err)
		}
	})
	if !strings.Contains(output, "envoyage shim status") {
		t.Fatalf("shim help output = %q, want shim usage", output)
	}
}

func TestShimRejectsUnknownCommandAndExtraArgs(t *testing.T) {
	err := runShim([]string{"unknown"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runShim(unknown) error = nil, want error")
	}
	if !strings.Contains(err.Error(), `unknown shim command "unknown"`) {
		t.Fatalf("error = %q, want unknown command", err.Error())
	}

	err = runShim([]string{"status", "extra"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runShim(status extra) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "shim status does not accept arguments") {
		t.Fatalf("error = %q, want extra args error", err.Error())
	}
}

func TestShimRejectsExtraArgsForInstallAndUninstall(t *testing.T) {
	tests := [][]string{
		{"install", "extra"},
		{"uninstall", "extra"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			err := runShim(args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("runShim() error = nil, want extra args error")
			}
			if !strings.Contains(err.Error(), "does not accept arguments") {
				t.Fatalf("error = %q, want extra args error", err.Error())
			}
		})
	}
}

func TestShimPathHelpers(t *testing.T) {
	dir := t.TempDir()
	restoreShimHooks(t)
	userHomeDir = func() (string, error) { return dir, nil }

	got, err := expandHomePath("~")
	if err != nil {
		t.Fatalf("expandHomePath(~) error = %v", err)
	}
	if got != dir {
		t.Fatalf("expandHomePath(~) = %q, want %q", got, dir)
	}

	got, err = expandHomePath("~/bin")
	if err != nil {
		t.Fatalf("expandHomePath(~/bin) error = %v", err)
	}
	if got != filepath.Join(dir, "bin") {
		t.Fatalf("expandHomePath(~/bin) = %q, want home bin", got)
	}

	_, err = dockerShimPath("")
	if err == nil {
		t.Fatal("dockerShimPath(empty) error = nil, want error")
	}
}

func TestShimExecutablePathReportsErrors(t *testing.T) {
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return "", os.ErrPermission }

	_, err := envoyageExecutablePath()
	if err == nil {
		t.Fatal("envoyageExecutablePath() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "resolve envoyage executable") {
		t.Fatalf("error = %q, want executable resolution", err.Error())
	}
}

func TestRunPrintsVersion(t *testing.T) {
	tests := [][]string{
		{"version"},
		{"--version"},
		{"-v"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			output := captureStdout(t, func() {
				if err := run(args); err != nil {
					t.Fatalf("run(%v) error = %v", args, err)
				}
			})

			if output != "envoyage 0.3.0\n" {
				t.Fatalf("version output = %q, want envoyage 0.3.0", output)
			}
		})
	}
}

func TestRunVersionRejectsExtraArgs(t *testing.T) {
	err := run([]string{"version", "extra"})
	if err == nil {
		t.Fatal("run(version extra) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "version does not accept arguments") {
		t.Fatalf("error = %q, want extra argument error", err.Error())
	}
}

func TestRunEnvExtractAndInline(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(`services:
  app:
    environment:
      APP_ENV: production
      DB_PASSWORD: secret-password
`), 0o600); err != nil {
		t.Fatalf("WriteFile(compose) error = %v", err)
	}

	var dryRunOut bytes.Buffer
	if err := runEnv([]string{"extract"}, &dryRunOut); err != nil {
		t.Fatalf("runEnv(extract) error = %v", err)
	}
	if !strings.Contains(dryRunOut.String(), "dry-run") {
		t.Fatalf("extract output = %q, want dry-run guidance", dryRunOut.String())
	}
	if strings.Contains(dryRunOut.String(), "secret-password") {
		t.Fatalf("extract output leaked secret: %q", dryRunOut.String())
	}

	var extractOut bytes.Buffer
	if err := runEnv([]string{"extract", "--write"}, &extractOut); err != nil {
		t.Fatalf("runEnv(extract write) error = %v", err)
	}
	if !strings.Contains(extractOut.String(), ".secrets.env") {
		t.Fatalf("extract write output = %q, want secrets file section", extractOut.String())
	}

	var inlineOut bytes.Buffer
	if err := runEnv([]string{"inline", "--out", "compose.inline.yaml"}, &inlineOut); err != nil {
		t.Fatalf("runEnv(inline) error = %v", err)
	}
	if !strings.Contains(inlineOut.String(), "compose updates") {
		t.Fatalf("inline output = %q, want compose updates", inlineOut.String())
	}
	rendered, err := os.ReadFile(filepath.Join(dir, "compose.inline.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(rendered) error = %v", err)
	}
	if !strings.Contains(string(rendered), "DB_PASSWORD: secret-password") {
		t.Fatalf("rendered compose = %q, want inlined secret", rendered)
	}
}

func TestRunEnvRejectsInvalidUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "unknown",
			args: []string{"unknown"},
			want: `unknown env command "unknown"`,
		},
		{
			name: "extract positional",
			args: []string{"extract", "extra"},
			want: "env extract does not accept positional arguments",
		},
		{
			name: "inline missing out",
			args: []string{"inline"},
			want: "--out is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runEnv(tt.args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("runEnv() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRunCompletionPrintsScripts(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{
			shell: "bash",
			want:  []string{"complete -F _envoyage envoyage", "compose completion decrypt encrypt", "--system --bin-dir --lib-dir"},
		},
		{
			shell: "zsh",
			want:  []string{"#compdef envoyage", "'completion:generate shell completion script'"},
		},
		{
			shell: "fish",
			want:  []string{"complete -c envoyage", "-a \"bash zsh fish powershell\"", "-l system"},
		},
		{
			shell: "powershell",
			want:  []string{"Register-ArgumentCompleter", "'completion'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			var out bytes.Buffer
			if err := runCompletion([]string{tt.shell}, &out); err != nil {
				t.Fatalf("runCompletion(%q) error = %v", tt.shell, err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("completion output for %s missing %q:\n%s", tt.shell, want, out.String())
				}
			}
		})
	}
}

func TestRunCompletionRejectsInvalidArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing shell",
			args: nil,
			want: "completion requires one shell",
		},
		{
			name: "extra arg",
			args: []string{"bash", "extra"},
			want: "completion requires one shell",
		},
		{
			name: "unsupported shell",
			args: []string{"cmd"},
			want: `unsupported completion shell "cmd"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCompletion(tt.args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("runCompletion() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRunDispatchesCompletion(t *testing.T) {
	output := captureStdout(t, func() {
		if err := run([]string{"completion", "bash"}); err != nil {
			t.Fatalf("run(completion bash) error = %v", err)
		}
	})
	if !strings.Contains(output, "complete -F _envoyage envoyage") {
		t.Fatalf("completion output = %q, want bash completion script", output)
	}
}

func TestRunPrintsUsageForHelp(t *testing.T) {
	output := captureStdout(t, func() {
		if err := run([]string{"--help"}); err != nil {
			t.Fatalf("run(--help) error = %v", err)
		}
	})

	if !strings.Contains(output, "Envoyage") {
		t.Fatalf("help output = %q, want Envoyage", output)
	}
	if !strings.Contains(output, "envoyage compose") {
		t.Fatalf("help output = %q, want compose usage", output)
	}
}

func TestRunDispatchesSubcommandErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "compose",
			args: []string{"compose", "--env-file"},
			want: "--env-file requires a value",
		},
		{
			name: "encrypt",
			args: []string{"encrypt"},
			want: "read input env file",
		},
		{
			name: "keygen",
			args: []string{"keygen", "--bad-flag"},
			want: "flag provided but not defined",
		},
		{
			name: "decrypt",
			args: []string{"decrypt"},
			want: "read encrypted env file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if err == nil {
				t.Fatal("run() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestEncryptCommandValidatesRequiredFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing default input",
			args: nil,
			want: "read input env file",
		},
		{
			name: "bad flag",
			args: []string{"--bad-flag"},
			want: "flag provided but not defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runEncrypt(tt.args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("runEncrypt() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func restoreKeygenPermissionHooks(t *testing.T) {
	t.Helper()

	oldSystemIdentityFile := systemIdentityFile
	oldGetEUID := getEUID
	oldMkdirAll := mkdirAll
	oldChmod := chmod
	oldChown := chown
	oldLookupDockerGroupID := lookupDockerGroupID

	t.Cleanup(func() {
		systemIdentityFile = oldSystemIdentityFile
		getEUID = oldGetEUID
		mkdirAll = oldMkdirAll
		chmod = oldChmod
		chown = oldChown
		lookupDockerGroupID = oldLookupDockerGroupID
	})
}

func restoreShimHooks(t *testing.T) {
	t.Helper()

	oldExecutable := osExecutable
	oldUserHomeDir := userHomeDir
	t.Cleanup(func() {
		osExecutable = oldExecutable
		userHomeDir = oldUserHomeDir
	})
}

func restoreSystemInstallDefaults(t *testing.T, binDir string, libDir string) {
	t.Helper()

	oldBinDir := defaultSystemInstallBinDir
	oldLibDir := defaultSystemInstallLibDir
	defaultSystemInstallBinDir = binDir
	defaultSystemInstallLibDir = libDir
	t.Cleanup(func() {
		defaultSystemInstallBinDir = oldBinDir
		defaultSystemInstallLibDir = oldLibDir
	})
}

func restoreSystemShimDefault(t *testing.T, binDir string) {
	t.Helper()

	oldBinDir := defaultSystemShimBinDir
	defaultSystemShimBinDir = binDir
	t.Cleanup(func() {
		defaultSystemShimBinDir = oldBinDir
	})
}

func assertPathsRemoved(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after uninstall: %v", path, err)
		}
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("%s should exist: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}

func assertSymlinkTarget(t *testing.T, path string, want string) {
	t.Helper()

	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("Readlink(%s) error = %v", path, err)
	}
	if target != want {
		t.Fatalf("%s target = %q, want %q", path, target, want)
	}
}

func assertContains(t *testing.T, got string, want string) {
	t.Helper()

	if !strings.Contains(got, want) {
		t.Fatalf("%q does not contain %q", got, want)
	}
}

func assertOutputOrder(t *testing.T, output string, first string, second string) {
	t.Helper()

	firstIndex := strings.Index(output, first)
	secondIndex := strings.Index(output, second)
	if firstIndex < 0 || secondIndex < 0 || firstIndex > secondIndex {
		t.Fatalf("output = %q, want %q before %q", output, first, second)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("Close(stdout writer) error = %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll(stdout) error = %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close(stdout reader) error = %v", err)
	}
	return string(out)
}

const captureDockerScript = `#!/bin/sh
printf '{"args":[' > "$CAPTURE"
first=1
for arg in "$@"; do
  if [ "$first" = 1 ]; then first=0; else printf ',' >> "$CAPTURE"; fi
  printf '"%s"' "$arg" >> "$CAPTURE"
done
printf '],"token":"%s"}' "$TOKEN" >> "$CAPTURE"
`

type dockerCapture struct {
	Args  []string `json:"args"`
	Token string   `json:"token"`
}

func readDockerCapture(t *testing.T, path string) dockerCapture {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}

	var capture dockerCapture
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatalf("Unmarshal(capture) error = %v; data=%s", err, data)
	}
	return capture
}
