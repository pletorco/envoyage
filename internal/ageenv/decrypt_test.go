package ageenv

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
)

func TestDecryptAgeDotenv(t *testing.T) {
	dir := t.TempDir()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity() error = %v", err)
	}
	identityPath := filepath.Join(dir, "age-key.txt")
	if err := os.WriteFile(identityPath, []byte(identity.String()+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, identity.Recipient())
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if _, err := writer.Write([]byte("DB_PASSWORD=secret-password\nAPI_KEY=\"secret-api-key\"\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	got, err := Decrypt(encrypted.Bytes(), identityPath)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if got["DB_PASSWORD"] != "secret-password" {
		t.Fatalf("DB_PASSWORD = %q, want secret-password", got["DB_PASSWORD"])
	}
	if got["API_KEY"] != "secret-api-key" {
		t.Fatalf("API_KEY = %q, want secret-api-key", got["API_KEY"])
	}
}

func TestDecryptRequiresIdentity(t *testing.T) {
	_, err := Decrypt([]byte("not age"), "")
	if err == nil {
		t.Fatal("Decrypt() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "AGE_IDENTITY_FILE") {
		t.Fatalf("error = %q, want AGE_IDENTITY_FILE guidance", err.Error())
	}
}

func TestDecryptReportsIdentityAndCiphertextErrors(t *testing.T) {
	dir := t.TempDir()

	_, err := Decrypt([]byte("not age"), filepath.Join(dir, "missing-key.txt"))
	if err == nil {
		t.Fatal("Decrypt() missing identity error = nil, want error")
	}
	if !strings.Contains(err.Error(), "read identity file") {
		t.Fatalf("missing identity error = %q, want read identity file", err.Error())
	}

	badIdentityPath := filepath.Join(dir, "bad-key.txt")
	if err := os.WriteFile(badIdentityPath, []byte("not-an-age-identity\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(bad identity) error = %v", err)
	}
	_, err = Decrypt([]byte("not age"), badIdentityPath)
	if err == nil {
		t.Fatal("Decrypt() bad identity error = nil, want error")
	}
	if !strings.Contains(err.Error(), "parse identity file") {
		t.Fatalf("bad identity error = %q, want parse identity file", err.Error())
	}

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity() error = %v", err)
	}
	identityPath := filepath.Join(dir, "age-key.txt")
	if err := os.WriteFile(identityPath, []byte(identity.String()+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(identity) error = %v", err)
	}
	_, err = Decrypt([]byte("not age"), identityPath)
	if err == nil {
		t.Fatal("Decrypt() invalid ciphertext error = nil, want error")
	}
	if !strings.Contains(err.Error(), "decrypt age env file") {
		t.Fatalf("invalid ciphertext error = %q, want decrypt age env file", err.Error())
	}
}

func TestDecryptParseErrorDoesNotLeakSecretValue(t *testing.T) {
	dir := t.TempDir()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity() error = %v", err)
	}
	identityPath := filepath.Join(dir, "age-key.txt")
	if err := os.WriteFile(identityPath, []byte(identity.String()+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(identity) error = %v", err)
	}

	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, identity.Recipient())
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if _, err := writer.Write([]byte("API_KEY=\"super-secret\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err = Decrypt(encrypted.Bytes(), identityPath)
	if err == nil {
		t.Fatal("Decrypt() error = nil, want parse error")
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("decrypt parse error leaked secret: %v", err)
	}
	if !strings.Contains(err.Error(), "API_KEY") {
		t.Fatalf("decrypt parse error = %q, want key name", err.Error())
	}
}

func TestGenerateIdentityFileAndEncryptFile(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "age-key.txt")
	envPath := filepath.Join(dir, ".env")
	encryptedPath := filepath.Join(dir, ".env.age")

	generated, err := GenerateIdentityFile(identityPath, false)
	if err != nil {
		t.Fatalf("GenerateIdentityFile() error = %v", err)
	}
	if generated.IdentityPath != identityPath {
		t.Fatalf("IdentityPath = %q, want %q", generated.IdentityPath, identityPath)
	}
	if !strings.HasPrefix(generated.Recipient, "age1") {
		t.Fatalf("Recipient = %q, want age1 prefix", generated.Recipient)
	}

	info, err := os.Stat(identityPath)
	if err != nil {
		t.Fatalf("Stat(identity) error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("identity permissions = %v, want 0600", info.Mode().Perm())
	}

	if err := os.WriteFile(envPath, []byte("TOKEN=secret-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	if err := EncryptFile(EncryptOptions{
		InputPath:      envPath,
		OutputPath:     encryptedPath,
		IdentityPaths:  []string{identityPath},
		ForceOverwrite: false,
	}); err != nil {
		t.Fatalf("EncryptFile() error = %v", err)
	}

	encrypted, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(encrypted) error = %v", err)
	}
	if bytes.Contains(encrypted, []byte("secret-token")) {
		t.Fatal("encrypted output contains plaintext secret")
	}

	decrypted, err := Decrypt(encrypted, identityPath)
	if err != nil {
		t.Fatalf("Decrypt(encrypted) error = %v", err)
	}
	if decrypted["TOKEN"] != "secret-token" {
		t.Fatalf("TOKEN = %q, want secret-token", decrypted["TOKEN"])
	}
}

func TestDecryptFileRefusesOverwriteUnlessForced(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "age-key.txt")
	envPath := filepath.Join(dir, ".secrets.env")
	encryptedPath := filepath.Join(dir, ".env.age")
	outputPath := filepath.Join(dir, "decrypted.env")

	generated, err := GenerateIdentityFile(identityPath, false)
	if err != nil {
		t.Fatalf("GenerateIdentityFile() error = %v", err)
	}
	if generated.Recipient == "" {
		t.Fatal("generated recipient is empty")
	}
	plaintext := []byte("TOKEN=secret-token\n# keep formatting\n")
	if err := os.WriteFile(envPath, plaintext, 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	if err := EncryptFile(EncryptOptions{
		InputPath:     envPath,
		OutputPath:    encryptedPath,
		IdentityPaths: []string{identityPath},
	}); err != nil {
		t.Fatalf("EncryptFile() error = %v", err)
	}
	if err := os.WriteFile(outputPath, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing output) error = %v", err)
	}

	err = DecryptFile(DecryptOptions{
		InputPath:    encryptedPath,
		OutputPath:   outputPath,
		IdentityPath: identityPath,
	})
	if err == nil {
		t.Fatal("DecryptFile() error = nil, want overwrite error")
	}
	existing, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(output) error = %v", err)
	}
	if string(existing) != "existing\n" {
		t.Fatalf("output was overwritten without force: %q", existing)
	}

	if err := DecryptFile(DecryptOptions{
		InputPath:      encryptedPath,
		OutputPath:     outputPath,
		IdentityPath:   identityPath,
		ForceOverwrite: true,
	}); err != nil {
		t.Fatalf("DecryptFile(force) error = %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(output force) error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypted output = %q, want %q", got, plaintext)
	}
}

func TestGenerateIdentityFileRefusesOverwriteUnlessForced(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "age-key.txt")
	if err := os.WriteFile(identityPath, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}

	_, err := GenerateIdentityFile(identityPath, false)
	if err == nil {
		t.Fatal("GenerateIdentityFile() error = nil, want overwrite error")
	}

	data, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatalf("ReadFile(identity) error = %v", err)
	}
	if string(data) != "existing\n" {
		t.Fatalf("identity file was overwritten without force: %q", data)
	}

	generated, err := GenerateIdentityFile(identityPath, true)
	if err != nil {
		t.Fatalf("GenerateIdentityFile(force) error = %v", err)
	}
	if generated.Recipient == "" {
		t.Fatal("Recipient is empty after forced key generation")
	}
}

func TestEncryptFileWithRecipientAndOverwriteProtection(t *testing.T) {
	dir := t.TempDir()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity() error = %v", err)
	}
	envPath := filepath.Join(dir, ".env")
	encryptedPath := filepath.Join(dir, ".env.age")
	if err := os.WriteFile(envPath, []byte("TOKEN=secret-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	if err := os.WriteFile(encryptedPath, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing encrypted) error = %v", err)
	}

	err = EncryptFile(EncryptOptions{
		InputPath:  envPath,
		OutputPath: encryptedPath,
		Recipients: []string{identity.Recipient().String()},
	})
	if err == nil {
		t.Fatal("EncryptFile() error = nil, want overwrite error")
	}
	data, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(encrypted) error = %v", err)
	}
	if string(data) != "existing\n" {
		t.Fatalf("encrypted file was overwritten without force: %q", data)
	}

	if err := EncryptFile(EncryptOptions{
		InputPath:      envPath,
		OutputPath:     encryptedPath,
		Recipients:     []string{identity.Recipient().String()},
		ForceOverwrite: true,
	}); err != nil {
		t.Fatalf("EncryptFile(force) error = %v", err)
	}
	encrypted, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(encrypted force) error = %v", err)
	}
	if bytes.Contains(encrypted, []byte("secret-token")) {
		t.Fatal("encrypted output contains plaintext secret")
	}
}

func TestEncryptFileValidatesRequiredInputs(t *testing.T) {
	tests := []struct {
		name string
		opts EncryptOptions
		want string
	}{
		{
			name: "missing input",
			opts: EncryptOptions{OutputPath: "out.age", Recipients: []string{"age1notvalid"}},
			want: "--in is required",
		},
		{
			name: "missing output",
			opts: EncryptOptions{InputPath: ".env", Recipients: []string{"age1notvalid"}},
			want: "--out is required",
		},
		{
			name: "missing recipient",
			opts: EncryptOptions{InputPath: ".env", OutputPath: ".env.age"},
			want: "at least one --recipient or --identity is required",
		},
		{
			name: "invalid recipient",
			opts: EncryptOptions{InputPath: ".env", OutputPath: ".env.age", Recipients: []string{"not-a-recipient"}},
			want: "parse recipient",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EncryptFile(tt.opts)
			if err == nil {
				t.Fatal("EncryptFile() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}
