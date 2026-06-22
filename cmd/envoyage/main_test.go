package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
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

			if output != "envoyage 0.1.0\n" {
				t.Fatalf("version output = %q, want envoyage 0.1.0", output)
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
