package compose

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/swoogi/envoyage/internal/ageenv"
	"github.com/swoogi/envoyage/internal/dotenv"
)

type Options struct {
	EnvFiles     []string
	IdentityFile string
	ComposeArgs  []string
}

const DefaultIdentityFile = "/etc/envoyage/envoyage-key.txt"

var defaultIdentityFile = DefaultIdentityFile
var defaultEnvFiles = []string{".env", ".env.age"}

func ParseArgs(args []string) (Options, error) {
	var opts Options

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--env-file":
			if i+1 >= len(args) {
				return Options{}, fmt.Errorf("--env-file requires a value")
			}
			i++
			opts.EnvFiles = append(opts.EnvFiles, args[i])
		case strings.HasPrefix(arg, "--env-file="):
			opts.EnvFiles = append(opts.EnvFiles, strings.TrimPrefix(arg, "--env-file="))
		case arg == "--identity":
			if i+1 >= len(args) {
				return Options{}, fmt.Errorf("--identity requires a value")
			}
			i++
			opts.IdentityFile = args[i]
		case strings.HasPrefix(arg, "--identity="):
			opts.IdentityFile = strings.TrimPrefix(arg, "--identity=")
		default:
			opts.ComposeArgs = append(opts.ComposeArgs, arg)
		}
	}

	if opts.IdentityFile == "" {
		opts.IdentityFile = os.Getenv("AGE_IDENTITY_FILE")
	}
	if opts.IdentityFile == "" {
		opts.IdentityFile = defaultIdentityFile
	}
	if len(opts.EnvFiles) == 0 {
		envFiles, err := existingDefaultEnvFiles(defaultEnvFiles)
		if err != nil {
			return Options{}, err
		}
		opts.EnvFiles = envFiles
	}

	return opts, nil
}

func existingDefaultEnvFiles(paths []string) ([]string, error) {
	envFiles := make([]string, 0, len(paths))
	for _, path := range paths {
		_, err := os.Stat(path)
		switch {
		case err == nil:
			envFiles = append(envFiles, path)
		case os.IsNotExist(err):
			continue
		default:
			return nil, fmt.Errorf("stat default env file %s: %w", path, err)
		}
	}
	return envFiles, nil
}

func LoadEnvFiles(paths []string, identityPath string) (map[string]string, error) {
	merged := make(map[string]string)

	for _, path := range paths {
		env, err := loadEnvFile(path, identityPath)
		if err != nil {
			return nil, err
		}
		for key, value := range env {
			merged[key] = value
		}
	}

	return merged, nil
}

func loadEnvFile(path string, identityPath string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}

	if strings.HasSuffix(path, ".age") {
		env, err := ageenv.Decrypt(data, identityPath)
		if err != nil {
			return nil, fmt.Errorf("load encrypted env file %s: %w", path, err)
		}
		return env, nil
	}

	env, err := dotenv.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse env file %s: %w", path, err)
	}
	return env, nil
}

func MergeEnv(base []string, overlay map[string]string) []string {
	merged := make(map[string]string, len(base)+len(overlay))
	order := make([]string, 0, len(base)+len(overlay))

	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, exists := merged[key]; !exists {
			order = append(order, key)
		}
		merged[key] = value
	}

	for key, value := range overlay {
		if _, exists := merged[key]; !exists {
			order = append(order, key)
		}
		merged[key] = value
	}

	out := make([]string, 0, len(merged))
	for _, key := range order {
		out = append(out, key+"="+merged[key])
	}
	return out
}
