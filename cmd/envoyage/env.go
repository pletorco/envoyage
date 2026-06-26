package main

import (
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/swoogi/envoyage/internal/envops"
)

func runEnv(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printEnvUsage(stdout)
		return nil
	}

	switch args[0] {
	case "extract":
		return runEnvExtract(args[1:], stdout)
	case "inline":
		return runEnvInline(args[1:], stdout)
	default:
		return fmt.Errorf("unknown env command %q", args[0])
	}
}

func runEnvExtract(args []string, stdout io.Writer) error {
	var composePath string
	var envPath string
	var secretsPath string
	var write bool
	var secrets bool

	flags := flag.NewFlagSet("env extract", flag.ContinueOnError)
	flags.StringVar(&composePath, "compose", "", "compose file path")
	flags.StringVar(&envPath, "env", envops.DefaultEnvPath, "dotenv output path for non-secret values")
	flags.StringVar(&secretsPath, "secrets-env", envops.DefaultSecretsPath, "dotenv output path for secret-looking values")
	flags.BoolVar(&write, "write", false, "write env files and update the compose file")
	flags.BoolVar(&secrets, "secrets", true, "split secret-looking keys into .secrets.env")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("env extract does not accept positional arguments")
	}

	result, err := envops.Extract(envops.ExtractOptions{
		ComposePath: composePath,
		EnvPath:     envPath,
		SecretsPath: secretsPath,
		Write:       write,
		Secrets:     secrets,
	})
	if err != nil {
		return err
	}
	printEnvOperationResult(stdout, "extract", result, write)
	return nil
}

func runEnvInline(args []string, stdout io.Writer) error {
	var composePath string
	var outPath string
	var envFiles stringList
	var identityPath string
	var force bool

	flags := flag.NewFlagSet("env inline", flag.ContinueOnError)
	flags.StringVar(&composePath, "compose", "", "compose file path")
	flags.StringVar(&outPath, "out", "", "rendered compose output path")
	flags.StringVar(&outPath, "o", "", "rendered compose output path")
	flags.Var(&envFiles, "env-file", "dotenv or age env file, can be repeated")
	flags.StringVar(&identityPath, "identity", "", "age identity path")
	flags.StringVar(&identityPath, "i", "", "age identity path")
	flags.BoolVar(&force, "force", false, "overwrite output file if it exists")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("env inline does not accept positional arguments")
	}
	if identityPath == "" {
		identityPath = defaultEncryptIdentityPath()
	}

	result, err := envops.Inline(envops.InlineOptions{
		ComposePath:  composePath,
		OutputPath:   outPath,
		EnvFiles:     envFiles,
		IdentityFile: identityPath,
		Force:        force,
	})
	if err != nil {
		return err
	}
	printEnvOperationResult(stdout, "inline", result, true)
	return nil
}

func printEnvOperationResult(stdout io.Writer, operation string, result envops.Result, wrote bool) {
	fmt.Fprintf(stdout, "%s: %s\n", operation, result.ComposePath)
	if len(result.EnvKeys) > 0 {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, ".env:")
		printKeyList(stdout, result.EnvKeys)
	}
	if len(result.SecretKeys) > 0 {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, ".secrets.env:")
		printKeyList(stdout, result.SecretKeys)
	}
	if len(result.Updates) > 0 {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "compose updates:")
		for _, update := range sortedUpdates(result.Updates) {
			fmt.Fprintf(stdout, "  services.%s.environment.%s\n", update.Service, update.Key)
		}
	}
	if !wrote {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "dry-run: pass --write to update files")
	}
}

func printKeyList(stdout io.Writer, keys []string) {
	for _, key := range keys {
		fmt.Fprintf(stdout, "  %s\n", key)
	}
}

func sortedUpdates(updates []envops.Update) []envops.Update {
	out := append([]envops.Update(nil), updates...)
	sort.Slice(out, func(i int, j int) bool {
		if out[i].Service == out[j].Service {
			return out[i].Key < out[j].Key
		}
		return out[i].Service < out[j].Service
	})
	return out
}

func printEnvUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, `Usage:
  envoyage env extract [--compose compose.yaml] [--write]
  envoyage env inline --out compose.inline.yaml [--compose compose.yaml]

Commands:
  extract   Move fixed Compose environment values into .env and .secrets.env
  inline    Write a rendered Compose file with env values inlined`)
}
