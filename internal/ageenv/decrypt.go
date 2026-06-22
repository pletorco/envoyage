package ageenv

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
	"github.com/swoogi/envoyage/internal/dotenv"
)

type DecryptOptions struct {
	InputPath      string
	OutputPath     string
	IdentityPath   string
	ForceOverwrite bool
}

// Decrypt decrypts age-encrypted dotenv data entirely in memory.
func Decrypt(ciphertext []byte, identityPath string) (map[string]string, error) {
	plaintext, err := DecryptBytes(ciphertext, identityPath)
	if err != nil {
		return nil, err
	}

	env, err := dotenv.Parse(bytes.NewReader(plaintext))
	if err != nil {
		return nil, fmt.Errorf("parse decrypted env file: %w", err)
	}
	return env, nil
}

// DecryptBytes decrypts age-encrypted data entirely in memory.
func DecryptBytes(ciphertext []byte, identityPath string) ([]byte, error) {
	identities, err := loadIdentities(identityPath)
	if err != nil {
		return nil, err
	}

	reader, err := age.Decrypt(bytes.NewReader(ciphertext), identities...)
	if err != nil {
		return nil, fmt.Errorf("decrypt age env file: %w", err)
	}

	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read decrypted env file: %w", err)
	}
	return plaintext, nil
}

// DecryptFile decrypts an age-encrypted file to a plaintext file.
func DecryptFile(opts DecryptOptions) error {
	if opts.InputPath == "" {
		return fmt.Errorf("--in is required")
	}
	if opts.OutputPath == "" {
		return fmt.Errorf("--out is required")
	}

	ciphertext, err := os.ReadFile(opts.InputPath)
	if err != nil {
		return fmt.Errorf("read encrypted env file %s: %w", opts.InputPath, err)
	}

	plaintext, err := DecryptBytes(ciphertext, opts.IdentityPath)
	if err != nil {
		return err
	}

	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !opts.ForceOverwrite {
		flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(opts.OutputPath, flag, 0o600)
	if err != nil {
		return fmt.Errorf("create decrypted env file %s: %w", opts.OutputPath, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, bytes.NewReader(plaintext)); err != nil {
		return fmt.Errorf("write decrypted env file %s: %w", opts.OutputPath, err)
	}
	return nil
}

func loadIdentities(path string) ([]age.Identity, error) {
	if path == "" {
		return nil, fmt.Errorf("identity file required: pass --identity, set AGE_IDENTITY_FILE, or configure the default identity file")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read identity file %s: %w", path, err)
	}

	ids, err := age.ParseIdentities(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("parse identity file %s: %w", path, err)
	}
	return ids, nil
}
