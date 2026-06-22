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
	Env       []string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
}

func NewRunner() Runner {
	dockerBin := os.Getenv("ENVOYAGE_DOCKER_BIN")
	if dockerBin == "" {
		dockerBin = "docker"
	}

	return Runner{
		DockerBin: dockerBin,
		Env:       os.Environ(),
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	}
}

func NewShimRunner(invokedPath string) (Runner, error) {
	runner := NewRunner()
	if os.Getenv("ENVOYAGE_DOCKER_BIN") != "" {
		return runner, nil
	}

	dockerBin, err := FindRealDockerBin(invokedPath)
	if err != nil {
		return Runner{}, err
	}
	runner.DockerBin = dockerBin
	return runner, nil
}

func (r Runner) Run(ctx context.Context, args []string, env map[string]string) error {
	return r.run(ctx, append([]string{"compose"}, args...), env, "run docker compose")
}

func (r Runner) RunDocker(ctx context.Context, args []string, env map[string]string) error {
	return r.run(ctx, args, env, "run docker")
}

func (r Runner) run(ctx context.Context, args []string, env map[string]string, errorPrefix string) error {
	if r.DockerBin == "" {
		r.DockerBin = "docker"
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
	name := "docker"
	if runtime.GOOS == "windows" {
		name = "docker.exe"
	}

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

	return "", fmt.Errorf("real docker binary not found for shim mode; set ENVOYAGE_DOCKER_BIN=/path/to/docker")
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
