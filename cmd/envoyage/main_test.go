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
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if string(data) != "envoyage-binary\n" {
		t.Fatalf("installed binary = %q, want source content", data)
	}
	linkTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink(link) error = %v", err)
	}
	if linkTarget != target {
		t.Fatalf("link target = %q, want %q", linkTarget, target)
	}
	if !strings.Contains(installOut.String(), "installed envoyage") {
		t.Fatalf("install output = %q, want installed envoyage", installOut.String())
	}

	var statusOut bytes.Buffer
	if err := runStatus([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &statusOut); err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}
	if !strings.Contains(statusOut.String(), "installed binary: yes") {
		t.Fatalf("status output = %q, want installed binary yes", statusOut.String())
	}
	if !strings.Contains(statusOut.String(), "command symlink installed: yes") {
		t.Fatalf("status output = %q, want command symlink yes", statusOut.String())
	}

	var uninstallOut bytes.Buffer
	if err := runUninstall([]string{"--bin-dir", binDir, "--lib-dir", libDir}, &uninstallOut); err != nil {
		t.Fatalf("runUninstall() error = %v", err)
	}
	if !strings.Contains(uninstallOut.String(), "removed command symlink") {
		t.Fatalf("uninstall output = %q, want removed command symlink", uninstallOut.String())
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("command symlink still exists after uninstall: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("installed binary still exists after uninstall: %v", err)
	}
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
	target := filepath.Join(dir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return target, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	var installOut bytes.Buffer
	if err := runShim([]string{"install", "--bin-dir", binDir}, &installOut); err != nil {
		t.Fatalf("runShim(install) error = %v", err)
	}
	shimPath := filepath.Join(binDir, "docker")
	linkTarget, err := os.Readlink(shimPath)
	if err != nil {
		t.Fatalf("Readlink(shim) error = %v", err)
	}
	if linkTarget != target {
		t.Fatalf("shim target = %q, want %q", linkTarget, target)
	}
	if !strings.Contains(installOut.String(), "installed shim") {
		t.Fatalf("install output = %q, want installed shim", installOut.String())
	}

	var statusOut bytes.Buffer
	if err := runShim([]string{"status", "--bin-dir", binDir}, &statusOut); err != nil {
		t.Fatalf("runShim(status) error = %v", err)
	}
	if !strings.Contains(statusOut.String(), "installed: yes") {
		t.Fatalf("status output = %q, want installed yes", statusOut.String())
	}
	if !strings.Contains(statusOut.String(), "shim target: "+target) {
		t.Fatalf("status output = %q, want shim target", statusOut.String())
	}

	var secondInstallOut bytes.Buffer
	if err := runShim([]string{"install", "--bin-dir", binDir}, &secondInstallOut); err != nil {
		t.Fatalf("runShim(install existing) error = %v", err)
	}
	if !strings.Contains(secondInstallOut.String(), "shim already installed") {
		t.Fatalf("second install output = %q, want already installed", secondInstallOut.String())
	}

	var uninstallOut bytes.Buffer
	if err := runShim([]string{"uninstall", "--bin-dir", binDir}, &uninstallOut); err != nil {
		t.Fatalf("runShim(uninstall) error = %v", err)
	}
	if !strings.Contains(uninstallOut.String(), "removed shim") {
		t.Fatalf("uninstall output = %q, want removed shim", uninstallOut.String())
	}
	if _, err := os.Lstat(shimPath); !os.IsNotExist(err) {
		t.Fatalf("shim still exists after uninstall: %v", err)
	}
}

func TestShimInstallForceRecreatesExistingShim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	target := filepath.Join(dir, "envoyage")
	shimPath := filepath.Join(binDir, "docker")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return target, nil }

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	if err := os.Symlink(target, shimPath); err != nil {
		t.Fatalf("Symlink(existing shim) error = %v", err)
	}

	if err := runShim([]string{"install", "--bin-dir", binDir, "--force"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(install force) error = %v", err)
	}
	linkTarget, err := os.Readlink(shimPath)
	if err != nil {
		t.Fatalf("Readlink(shim) error = %v", err)
	}
	if linkTarget != target {
		t.Fatalf("shim target = %q, want %q", linkTarget, target)
	}
}

func TestShimUninstallMissingIsNoop(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return target, nil }

	var out bytes.Buffer
	if err := runShim([]string{"uninstall", "--bin-dir", filepath.Join(dir, "bin")}, &out); err != nil {
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
	target := filepath.Join(dir, "envoyage")
	shimPath := filepath.Join(binDir, "docker")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return target, nil }

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	if err := os.WriteFile(shimPath, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(existing docker) error = %v", err)
	}

	err := runShim([]string{"install", "--bin-dir", binDir, "--force"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runShim(install non-envoyage) error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite non-Envoyage docker") {
		t.Fatalf("install error = %q, want overwrite refusal", err.Error())
	}

	err = runShim([]string{"uninstall", "--bin-dir", binDir}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runShim(uninstall non-envoyage) error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "refusing to remove non-Envoyage docker") {
		t.Fatalf("uninstall error = %q, want remove refusal", err.Error())
	}
}

func TestShimStatusWithDockerBinEnv(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return target, nil }
	t.Setenv("ENVOYAGE_DOCKER_BIN", "/custom/docker")

	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	var out bytes.Buffer
	if err := runShim([]string{"status", "--bin-dir", filepath.Join(dir, "bin")}, &out); err != nil {
		t.Fatalf("runShim(status) error = %v", err)
	}
	if !strings.Contains(out.String(), "ENVOYAGE_DOCKER_BIN: /custom/docker") {
		t.Fatalf("status output = %q, want configured docker bin", out.String())
	}
}

func TestShimUsesDefaultHomeBinDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior requires platform-specific privileges on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "envoyage")
	restoreShimHooks(t)
	osExecutable = func() (string, error) { return target, nil }
	userHomeDir = func() (string, error) { return dir, nil }

	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	if err := runShim([]string{"install"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runShim(default install) error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".local", "bin", "docker")); err != nil {
		t.Fatalf("default shim was not installed: %v", err)
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

			if output != "envoyage 0.2.1\n" {
				t.Fatalf("version output = %q, want envoyage 0.2.1", output)
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
