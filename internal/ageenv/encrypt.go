package ageenv

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"filippo.io/age"
)

type EncryptOptions struct {
	InputPath      string
	OutputPath     string
	Recipients     []string
	IdentityPaths  []string
	ForceOverwrite bool
}

type GeneratedIdentity struct {
	IdentityPath string
	Recipient    string
}

// EncryptFile encrypts a dotenv file to age format without logging plaintext.
func EncryptFile(opts EncryptOptions) error {
	if opts.InputPath == "" {
		return fmt.Errorf("--in is required")
	}
	if opts.OutputPath == "" {
		return fmt.Errorf("--out is required")
	}

	plaintext, err := os.ReadFile(opts.InputPath)
	if err != nil {
		return fmt.Errorf("read input env file %s: %w", opts.InputPath, err)
	}

	recipients, err := loadRecipients(opts.Recipients, opts.IdentityPaths)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return fmt.Errorf("at least one --recipient or --identity is required")
	}

	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, recipients...)
	if err != nil {
		return fmt.Errorf("create age encryptor: %w", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		return fmt.Errorf("encrypt env file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish encrypted env file: %w", err)
	}

	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !opts.ForceOverwrite {
		flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(opts.OutputPath, flag, 0o600)
	if err != nil {
		return fmt.Errorf("create output env file %s: %w", opts.OutputPath, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, bytes.NewReader(encrypted.Bytes())); err != nil {
		return fmt.Errorf("write encrypted env file %s: %w", opts.OutputPath, err)
	}
	return nil
}

// GenerateIdentityFile writes a new age identity file and returns its recipient.
func GenerateIdentityFile(path string, forceOverwrite bool) (GeneratedIdentity, error) {
	if path == "" {
		return GeneratedIdentity{}, fmt.Errorf("--out is required")
	}

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return GeneratedIdentity{}, fmt.Errorf("generate age identity: %w", err)
	}

	flag := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if forceOverwrite {
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flag, 0o600)
	if err != nil {
		return GeneratedIdentity{}, fmt.Errorf("create identity file %s: %w", path, err)
	}
	defer file.Close()

	if _, err := fmt.Fprintf(file, "%s\n", identity.String()); err != nil {
		return GeneratedIdentity{}, fmt.Errorf("write identity file %s: %w", path, err)
	}

	return GeneratedIdentity{
		IdentityPath: path,
		Recipient:    identity.Recipient().String(),
	}, nil
}

func loadRecipients(recipientStrings []string, identityPaths []string) ([]age.Recipient, error) {
	recipients := make([]age.Recipient, 0, len(recipientStrings)+len(identityPaths))

	for _, recipient := range recipientStrings {
		parsed, err := age.ParseX25519Recipient(recipient)
		if err != nil {
			return nil, fmt.Errorf("parse recipient: %w", err)
		}
		recipients = append(recipients, parsed)
	}

	for _, path := range identityPaths {
		identities, err := loadIdentities(path)
		if err != nil {
			return nil, err
		}
		for _, identity := range identities {
			switch identity := identity.(type) {
			case interface{ Recipient() *age.X25519Recipient }:
				recipients = append(recipients, identity.Recipient())
			case interface{ Recipient() *age.HybridRecipient }:
				recipients = append(recipients, identity.Recipient())
			default:
				return nil, fmt.Errorf("identity file %s contains an identity that cannot provide a recipient", path)
			}
		}
	}

	return recipients, nil
}
