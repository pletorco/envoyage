package compose

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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

func (r Runner) Run(ctx context.Context, args []string, env map[string]string) error {
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

	cmd := exec.CommandContext(ctx, r.DockerBin, append([]string{"compose"}, args...)...)
	cmd.Env = MergeEnv(r.Env, env)
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run docker compose: %w", err)
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
