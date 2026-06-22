package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/swoogi/envoyage/internal/compose"
)

var (
	defaultShimBinDir = "~/.local/bin"
	osExecutable      = os.Executable
	userHomeDir       = os.UserHomeDir
)

func runShim(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printShimUsage(stdout)
		return nil
	}

	switch args[0] {
	case "install":
		return runShimInstall(args[1:], stdout)
	case "uninstall":
		return runShimUninstall(args[1:], stdout)
	case "status":
		return runShimStatus(args[1:], stdout)
	default:
		return fmt.Errorf("unknown shim command %q", args[0])
	}
}

func runShimInstall(args []string, stdout io.Writer) error {
	var binDir string
	var force bool

	flags := flag.NewFlagSet("shim install", flag.ContinueOnError)
	flags.StringVar(&binDir, "bin-dir", defaultShimBinDir, "directory where the docker shim symlink is installed")
	flags.BoolVar(&force, "force", false, "recreate an existing Envoyage-managed shim symlink")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("shim install does not accept arguments")
	}

	shimPath, err := dockerShimPath(binDir)
	if err != nil {
		return err
	}
	target, err := envoyageExecutablePath()
	if err != nil {
		return err
	}

	if err := mkdirAll(filepath.Dir(shimPath), 0o755); err != nil {
		return fmt.Errorf("create shim directory %s: %w", filepath.Dir(shimPath), err)
	}

	installed, err := isEnvoyageShim(shimPath, target)
	if err != nil {
		return err
	}
	if installed {
		if force {
			if err := os.Remove(shimPath); err != nil {
				return fmt.Errorf("remove existing shim %s: %w", shimPath, err)
			}
		} else {
			fmt.Fprintf(stdout, "shim already installed: %s -> %s\n", shimPath, target)
			printShimActivation(stdout, filepath.Dir(shimPath), "")
			return nil
		}
	} else {
		if _, err := os.Lstat(shimPath); err == nil {
			return fmt.Errorf("refusing to overwrite non-Envoyage docker at %s", shimPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect shim path %s: %w", shimPath, err)
		}
	}

	if err := os.Symlink(target, shimPath); err != nil {
		return fmt.Errorf("create docker shim %s -> %s: %w", shimPath, target, err)
	}

	fmt.Fprintf(stdout, "installed shim: %s -> %s\n", shimPath, target)
	printShimActivation(stdout, filepath.Dir(shimPath), "")
	return nil
}

func runShimUninstall(args []string, stdout io.Writer) error {
	var binDir string

	flags := flag.NewFlagSet("shim uninstall", flag.ContinueOnError)
	flags.StringVar(&binDir, "bin-dir", defaultShimBinDir, "directory where the docker shim symlink is installed")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("shim uninstall does not accept arguments")
	}

	shimPath, err := dockerShimPath(binDir)
	if err != nil {
		return err
	}
	target, err := envoyageExecutablePath()
	if err != nil {
		return err
	}

	installed, err := isEnvoyageShim(shimPath, target)
	if err != nil {
		return err
	}
	if !installed {
		if _, statErr := os.Lstat(shimPath); errors.Is(statErr, os.ErrNotExist) {
			fmt.Fprintf(stdout, "shim not installed: %s\n", shimPath)
			return nil
		}
		return fmt.Errorf("refusing to remove non-Envoyage docker at %s", shimPath)
	}

	if err := os.Remove(shimPath); err != nil {
		return fmt.Errorf("remove shim %s: %w", shimPath, err)
	}
	fmt.Fprintf(stdout, "removed shim: %s\n", shimPath)
	return nil
}

func runShimStatus(args []string, stdout io.Writer) error {
	var binDir string

	flags := flag.NewFlagSet("shim status", flag.ContinueOnError)
	flags.StringVar(&binDir, "bin-dir", defaultShimBinDir, "directory where the docker shim symlink is installed")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("shim status does not accept arguments")
	}

	shimPath, err := dockerShimPath(binDir)
	if err != nil {
		return err
	}
	target, err := envoyageExecutablePath()
	if err != nil {
		return err
	}
	installed, err := isEnvoyageShim(shimPath, target)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "shim path: %s\n", shimPath)
	fmt.Fprintf(stdout, "envoyage target: %s\n", target)
	if installed {
		fmt.Fprintln(stdout, "installed: yes")
	} else {
		fmt.Fprintln(stdout, "installed: no")
	}

	if linkTarget, ok := readSymlinkTarget(shimPath); ok {
		fmt.Fprintf(stdout, "shim target: %s\n", linkTarget)
	}

	if pathDocker, err := exec.LookPath("docker"); err == nil {
		fmt.Fprintf(stdout, "PATH docker: %s\n", pathDocker)
	} else {
		fmt.Fprintln(stdout, "PATH docker: not found")
	}

	dockerBin := os.Getenv("ENVOYAGE_DOCKER_BIN")
	if dockerBin == "" {
		fmt.Fprintln(stdout, "ENVOYAGE_DOCKER_BIN: not set")
		if realDocker, err := compose.FindRealDockerBin(shimPath); err == nil {
			fmt.Fprintf(stdout, "real docker candidate: %s\n", realDocker)
		} else {
			fmt.Fprintf(stdout, "real docker candidate: %v\n", err)
		}
	} else {
		fmt.Fprintf(stdout, "ENVOYAGE_DOCKER_BIN: %s\n", dockerBin)
	}

	if installed {
		printShimActivation(stdout, filepath.Dir(shimPath), dockerBin)
	}
	return nil
}

func dockerShimPath(binDir string) (string, error) {
	dir, err := expandHomePath(binDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "docker"), nil
}

func envoyageExecutablePath() (string, error) {
	executable, err := osExecutable()
	if err != nil {
		return "", fmt.Errorf("resolve envoyage executable: %w", err)
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve envoyage executable path %s: %w", executable, err)
	}
	return absolute, nil
}

func isEnvoyageShim(path string, target string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect shim path %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}

	linkTarget, err := os.Readlink(path)
	if err != nil {
		return false, fmt.Errorf("read shim symlink %s: %w", path, err)
	}
	resolved := linkTarget
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(path), resolved)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return false, fmt.Errorf("resolve shim symlink %s: %w", path, err)
	}
	return samePath(resolved, target), nil
}

func samePath(left string, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr == nil && rightErr == nil {
		return os.SameFile(leftInfo, rightInfo)
	}
	leftAbs, leftAbsErr := filepath.Abs(left)
	rightAbs, rightAbsErr := filepath.Abs(right)
	return leftAbsErr == nil && rightAbsErr == nil && leftAbs == rightAbs
}

func readSymlinkTarget(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", false
	}
	return target, true
}

func expandHomePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("--bin-dir is required")
	}
	if path == "~" {
		home, err := userHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return home, nil
	}
	if len(path) > 2 && path[:2] == "~/" {
		home, err := userHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func printShimActivation(stdout io.Writer, binDir string, dockerBin string) {
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "To activate shim mode:")
	fmt.Fprintf(stdout, "  export PATH=%q:$PATH\n", binDir)
	if dockerBin == "" {
		fmt.Fprintln(stdout, "  export ENVOYAGE_DOCKER_BIN=/usr/bin/docker")
	}
}

func printShimUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, `Usage:
  envoyage shim status [--bin-dir ~/.local/bin]
  envoyage shim install [--bin-dir ~/.local/bin] [--force]
  envoyage shim uninstall [--bin-dir ~/.local/bin]`)
}
