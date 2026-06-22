package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	osuser "os/user"
	"path/filepath"
	"strconv"

	"github.com/swoogi/envoyage/internal/ageenv"
	"github.com/swoogi/envoyage/internal/compose"
)

var (
	version                  = "0.2.0"
	defaultKeygenOutputPath  = compose.DefaultIdentityFile
	defaultEncryptInputPath  = ".secrets.env"
	defaultEncryptOutputPath = ".env.age"
	defaultDecryptInputPath  = ".env.age"
	defaultDecryptOutputPath = ".secrets.env"
	systemIdentityFile       = compose.DefaultIdentityFile
	getEUID                  = os.Geteuid
	mkdirAll                 = os.MkdirAll
	chmod                    = os.Chmod
	chown                    = os.Chown
	lookupDockerGroupID      = dockerGroupID
)

func main() {
	if err := runForProgram(os.Args[0], os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "envoyage: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	return runForProgram("envoyage", args)
}

func runForProgram(program string, args []string) error {
	if isDockerShimName(filepath.Base(program)) {
		return runDockerShim(program, args)
	}
	return runEnvoyage(args)
}

func isDockerShimName(name string) bool {
	return name == "docker" || name == "docker.exe"
}

func runEnvoyage(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printUsage()
		return nil
	}
	if args[0] == "-v" || args[0] == "--version" {
		printVersion(os.Stdout)
		return nil
	}

	switch args[0] {
	case "compose":
		return compose.RunCompose(context.Background(), args[1:])
	case "encrypt":
		return runEncrypt(args[1:], os.Stdout)
	case "decrypt":
		return runDecrypt(args[1:], os.Stdout)
	case "keygen":
		return runKeygen(args[1:], os.Stdout)
	case "shim":
		return runShim(args[1:], os.Stdout)
	case "version":
		return runVersion(args[1:], os.Stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runDockerShim(program string, args []string) error {
	runner, err := compose.NewShimRunner(program)
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "compose" {
		return compose.RunComposeWithRunner(context.Background(), args[1:], runner)
	}
	return runner.RunDocker(context.Background(), args, nil)
}

func runVersion(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("version does not accept arguments")
	}

	printVersion(stdout)
	return nil
}

func printVersion(stdout io.Writer) {
	fmt.Fprintf(stdout, "envoyage %s\n", version)
}

func runEncrypt(args []string, stdout io.Writer) error {
	var recipients stringList
	var identityPaths stringList
	var inputPath string
	var outputPath string
	var force bool

	flags := flag.NewFlagSet("encrypt", flag.ContinueOnError)
	flags.StringVar(&inputPath, "in", "", "plaintext dotenv input path")
	flags.StringVar(&outputPath, "out", "", "encrypted age output path")
	flags.StringVar(&outputPath, "o", "", "encrypted age output path")
	flags.Var(&recipients, "recipient", "age recipient, can be repeated")
	flags.Var(&recipients, "r", "age recipient, can be repeated")
	flags.Var(&identityPaths, "identity", "age identity path to derive a recipient, can be repeated")
	flags.Var(&identityPaths, "i", "age identity path to derive a recipient, can be repeated")
	flags.BoolVar(&force, "force", false, "overwrite output file if it exists")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if inputPath == "" {
		inputPath = defaultEncryptInputPath
	}
	if outputPath == "" {
		outputPath = defaultEncryptOutputPath
	}
	if len(recipients) == 0 && len(identityPaths) == 0 {
		identityPaths = append(identityPaths, defaultEncryptIdentityPath())
	}

	if err := ageenv.EncryptFile(ageenv.EncryptOptions{
		InputPath:      inputPath,
		OutputPath:     outputPath,
		Recipients:     recipients,
		IdentityPaths:  identityPaths,
		ForceOverwrite: force,
	}); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "encrypted %s -> %s\n", inputPath, outputPath)
	return nil
}

func runDecrypt(args []string, stdout io.Writer) error {
	var inputPath string
	var outputPath string
	var identityPath string
	var force bool

	flags := flag.NewFlagSet("decrypt", flag.ContinueOnError)
	flags.StringVar(&inputPath, "in", "", "encrypted age input path")
	flags.StringVar(&outputPath, "out", "", "plaintext dotenv output path")
	flags.StringVar(&outputPath, "o", "", "plaintext dotenv output path")
	flags.StringVar(&identityPath, "identity", "", "age identity path")
	flags.StringVar(&identityPath, "i", "", "age identity path")
	flags.BoolVar(&force, "force", false, "overwrite output file if it exists")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if inputPath == "" {
		inputPath = defaultDecryptInputPath
	}
	if outputPath == "" {
		outputPath = defaultDecryptOutputPath
	}
	if identityPath == "" {
		identityPath = defaultEncryptIdentityPath()
	}

	if err := ageenv.DecryptFile(ageenv.DecryptOptions{
		InputPath:      inputPath,
		OutputPath:     outputPath,
		IdentityPath:   identityPath,
		ForceOverwrite: force,
	}); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "decrypted %s -> %s\n", inputPath, outputPath)
	return nil
}

func defaultEncryptIdentityPath() string {
	if path := os.Getenv("AGE_IDENTITY_FILE"); path != "" {
		return path
	}
	return defaultKeygenOutputPath
}

func runKeygen(args []string, stdout io.Writer) error {
	var outputPath string
	var force bool

	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	flags.StringVar(&outputPath, "out", "", "age identity output path")
	flags.StringVar(&outputPath, "o", "", "age identity output path")
	flags.BoolVar(&force, "force", false, "overwrite identity file if it exists")

	if err := flags.Parse(args); err != nil {
		return err
	}

	outProvided := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "out" || f.Name == "o" {
			outProvided = true
		}
	})
	if outputPath == "" {
		outputPath = defaultKeygenOutputPath
	}
	managedDefaultPath := !outProvided || outputPath == compose.DefaultIdentityFile

	if managedDefaultPath {
		if err := prepareDefaultKeygenOutput(outputPath); err != nil {
			return err
		}
	}

	generated, err := ageenv.GenerateIdentityFile(outputPath, force)
	if err != nil {
		return err
	}
	if managedDefaultPath {
		if err := applyDefaultKeygenPermissions(outputPath); err != nil {
			return err
		}
	}

	fmt.Fprintf(stdout, "identity: %s\nrecipient: %s\n", generated.IdentityPath, generated.Recipient)
	return nil
}

func prepareDefaultKeygenOutput(path string) error {
	if path == systemIdentityFile && getEUID() != 0 {
		return fmt.Errorf("default identity path %s requires root; rerun with sudo or pass --out PATH", path)
	}

	dir := filepath.Dir(path)
	if err := mkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create default identity directory %s: %w", dir, err)
	}
	if err := chmod(dir, 0o750); err != nil {
		return fmt.Errorf("set default identity directory permissions %s: %w", dir, err)
	}

	if path == systemIdentityFile && getEUID() == 0 {
		gid, ok := lookupDockerGroupID()
		if ok {
			if err := chown(dir, 0, gid); err != nil {
				return fmt.Errorf("set default identity directory owner %s: %w", dir, err)
			}
		} else {
			if err := chmod(dir, 0o700); err != nil {
				return fmt.Errorf("set root-only default identity directory permissions %s: %w", dir, err)
			}
		}
	}

	return nil
}

func applyDefaultKeygenPermissions(path string) error {
	mode := os.FileMode(0o640)
	if path == systemIdentityFile && getEUID() == 0 {
		if gid, ok := lookupDockerGroupID(); ok {
			if err := chown(path, 0, gid); err != nil {
				return fmt.Errorf("set default identity file owner %s: %w", path, err)
			}
		} else {
			mode = 0o600
			if err := chown(path, 0, 0); err != nil {
				return fmt.Errorf("set root-only default identity file owner %s: %w", path, err)
			}
		}
	}

	if err := chmod(path, mode); err != nil {
		return fmt.Errorf("set default identity file permissions %s: %w", path, err)
	}
	return nil
}

func dockerGroupID() (int, bool) {
	group, err := osuser.LookupGroup("docker")
	if err != nil {
		return 0, false
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return 0, false
	}
	return gid, true
}

func printUsage() {
	fmt.Fprintln(os.Stdout, `Envoyage - encrypted env-file wrapper for Docker Compose

Usage:
  envoyage compose [--identity PATH] [--env-file FILE...] [docker compose args...]
  envoyage keygen [--out age-key.txt]
  envoyage encrypt [--in .secrets.env] [--out .env.age] [--identity age-key.txt]
  envoyage encrypt [--in .secrets.env] [--out .env.age] --recipient age1...
  envoyage decrypt [--in .env.age] [--out .secrets.env] [--identity age-key.txt]
  envoyage shim status|install|uninstall
  envoyage version

Examples:
  envoyage version
  envoyage keygen
  envoyage keygen --out age-key.txt
  envoyage encrypt
  envoyage decrypt
  envoyage encrypt --in .secrets.env --out .env.age --identity age-key.txt
  envoyage compose up -d
  envoyage compose --identity ./age-key.txt config
  envoyage compose --env-file custom.env --env-file custom.env.age config
  envoyage shim status
  envoyage shim install --bin-dir ~/.local/bin

Environment:
  AGE_IDENTITY_FILE      age identity file path when --identity is omitted
  default identity       /etc/envoyage/envoyage-key.txt
  ENVOYAGE_DOCKER_BIN   docker binary path, defaults to docker`)
}

type stringList []string

func (s *stringList) String() string {
	return fmt.Sprint([]string(*s))
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}
