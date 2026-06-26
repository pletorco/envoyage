package compose

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type Runner struct {
	DockerBin string
	Runtime   string
	Env       []string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
}

func NewRunner() Runner {
	return NewRunnerForRuntime("docker")
}

func NewRunnerForRuntime(runtimeName string) Runner {
	runtimeName = normalizeRuntimeName(runtimeName)
	runtimeBin := os.Getenv(runtimeBinEnv(runtimeName))
	if runtimeBin == "" {
		runtimeBin = runtimeName
	}
	return Runner{
		DockerBin: runtimeBin,
		Runtime:   runtimeName,
		Env:       os.Environ(),
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	}
}

func NewShimRunner(invokedPath string) (Runner, error) {
	return NewShimRunnerForRuntime("docker", invokedPath)
}

func NewShimRunnerForRuntime(runtimeName string, invokedPath string) (Runner, error) {
	runtimeName = normalizeRuntimeName(runtimeName)
	runner := NewRunnerForRuntime(runtimeName)
	if os.Getenv(runtimeBinEnv(runtimeName)) != "" {
		return runner, nil
	}

	runtimeBin, err := FindRealRuntimeBin(runtimeName, invokedPath)
	if err != nil {
		return Runner{}, err
	}
	runner.DockerBin = runtimeBin
	return runner, nil
}

func (r Runner) Run(ctx context.Context, args []string, env map[string]string) error {
	runtimeName := r.runtimeName()
	return r.run(ctx, append([]string{"compose"}, args...), env, fmt.Sprintf("run %s compose", runtimeName))
}

func (r Runner) RunDocker(ctx context.Context, args []string, env map[string]string) error {
	runtimeName := r.runtimeName()
	return r.run(ctx, args, env, fmt.Sprintf("run %s", runtimeName))
}

func (r Runner) run(ctx context.Context, args []string, env map[string]string, errorPrefix string) error {
	if r.DockerBin == "" {
		r.DockerBin = r.runtimeName()
	}
	if r.Stdin == nil {
		r.Stdin = os.Stdin
	}
	if r.Stdout == nil {
		r.Stdout = io.Discard
	}
	if r.Stderr == nil {
		r.Stderr = io.Discard
	}

	cmd := exec.CommandContext(ctx, r.DockerBin, args...)
	cmd.Env = MergeEnv(r.Env, env)
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", errorPrefix, err)
	}
	return nil
}

func (r Runner) runtimeName() string {
	if r.Runtime != "" {
		return normalizeRuntimeName(r.Runtime)
	}
	if filepath.Base(r.DockerBin) == "podman" || filepath.Base(r.DockerBin) == "podman.exe" {
		return "podman"
	}
	return "docker"
}

func RunCompose(ctx context.Context, args []string) error {
	opts, err := ParseArgs(args)
	if err != nil {
		return err
	}

	env, err := LoadEnvFiles(opts.EnvFiles, opts.IdentityFile)
	if err != nil {
		return err
	}

	return NewRunner().Run(ctx, opts.ComposeArgs, env)
}

func RunComposeWithRunner(ctx context.Context, args []string, runner Runner) error {
	opts, err := ParseArgs(args)
	if err != nil {
		return err
	}

	env, err := LoadEnvFiles(opts.EnvFiles, opts.IdentityFile)
	if err != nil {
		return err
	}

	return runner.Run(ctx, opts.ComposeArgs, env)
}

func FindRealDockerBin(invokedPath string) (string, error) {
	return FindRealRuntimeBin("docker", invokedPath)
}

func FindRealRuntimeBin(runtimeName string, invokedPath string) (string, error) {
	runtimeName = normalizeRuntimeName(runtimeName)
	name := runtimeExecutableName(runtimeName)

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		if sameFile(candidate, invokedPath) || sameExecutable(candidate) {
			continue
		}
		return candidate, nil
	}

	return "", fmt.Errorf("real %s binary not found for shim mode; set %s=/path/to/%s", runtimeName, runtimeBinEnv(runtimeName), runtimeName)
}

func RuntimeBinEnv(runtimeName string) string {
	return runtimeBinEnv(runtimeName)
}

func runtimeBinEnv(runtimeName string) string {
	switch normalizeRuntimeName(runtimeName) {
	case "podman":
		return "ENVOYAGE_PODMAN_BIN"
	default:
		return "ENVOYAGE_DOCKER_BIN"
	}
}

func runtimeExecutableName(runtimeName string) string {
	name := normalizeRuntimeName(runtimeName)
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func normalizeRuntimeName(runtimeName string) string {
	if runtimeName == "podman" {
		return "podman"
	}
	return "docker"
}

func sameExecutable(path string) bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	return sameFile(path, executable)
}

func sameFile(left string, right string) bool {
	if left == "" || right == "" {
		return false
	}

	leftInfo, err := os.Stat(left)
	if err != nil {
		return false
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false
	}
	return os.SameFile(leftInfo, rightInfo)
}
