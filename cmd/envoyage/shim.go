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
	defaultShimBinDir       = "~/.local/bin"
	defaultSystemShimBinDir = "/usr/local/bin"
	osExecutable            = os.Executable
	userHomeDir             = os.UserHomeDir
)

const (
	shimBinDirFlag  = "bin-dir"
	shimBinDirUsage = "directory where the docker shim symlink is installed"
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
	var system bool

	flags := flag.NewFlagSet("shim install", flag.ContinueOnError)
	flags.StringVar(&binDir, shimBinDirFlag, defaultShimBinDir, shimBinDirUsage)
	flags.BoolVar(&force, "force", false, "recreate an existing Envoyage-managed shim symlink")
	flags.BoolVar(&system, "system", false, "install the docker shim to /usr/local/bin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("shim install does not accept arguments")
	}

	binDirProvided := flagProvided(flags, shimBinDirFlag)
	shimPath, err := dockerShimPathForMode(binDir, system, binDirProvided)
	if err != nil {
		return err
	}
	installPaths, err := shimEnvoyageInstallPaths(system)
	if err != nil {
		return err
	}
	if err := ensureShimEnvoyageInstall(installPaths, force, stdout); err != nil {
		return err
	}
	managedTargets := shimManagedTargets(installPaths.Target)

	if err := mkdirAll(filepath.Dir(shimPath), 0o755); err != nil {
		return fmt.Errorf("create shim directory %s: %w", filepath.Dir(shimPath), err)
	}

	shouldInstall, err := prepareShimInstall(shimPath, installPaths.Target, managedTargets, force, stdout)
	if err != nil {
		return err
	}
	if !shouldInstall {
		return nil
	}

	if err := os.Symlink(installPaths.Target, shimPath); err != nil {
		return fmt.Errorf("create docker shim %s -> %s: %w", shimPath, installPaths.Target, err)
	}

	fmt.Fprintf(stdout, "installed shim: %s -> %s\n", shimPath, installPaths.Target)
	printShimActivation(stdout, filepath.Dir(shimPath), "")
	return nil
}

func shimEnvoyageInstallPaths(system bool) (installPaths, error) {
	if system {
		return resolveInstallPaths(defaultSystemInstallBinDir, defaultSystemInstallLibDir)
	}
	return resolveInstallPaths(defaultInstallBinDir, defaultInstallLibDir)
}

func ensureShimEnvoyageInstall(paths installPaths, force bool, stdout io.Writer) error {
	source, err := envoyageExecutablePath()
	if err != nil {
		return err
	}
	if err := mkdirAll(paths.LibDir, 0o755); err != nil {
		return fmt.Errorf("create install directory %s: %w", paths.LibDir, err)
	}
	if err := mkdirAll(paths.BinDir, 0o755); err != nil {
		return fmt.Errorf("create command directory %s: %w", paths.BinDir, err)
	}

	info, err := os.Lstat(paths.Target)
	switch {
	case err == nil && info.IsDir():
		return fmt.Errorf("refusing to overwrite directory at install target %s", paths.Target)
	case err == nil && force:
		if err := installEnvoyageBinary(source, paths.Target, true); err != nil {
			return err
		}
	case errors.Is(err, os.ErrNotExist):
		if err := installEnvoyageBinary(source, paths.Target, false); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("inspect install target %s: %w", paths.Target, err)
	}

	if err := installEnvoyageSymlink(paths.Link, paths.Target, force, stdout); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "envoyage install target: %s\n", paths.Target)
	fmt.Fprintf(stdout, "envoyage command symlink: %s -> %s\n", paths.Link, paths.Target)
	return nil
}

func prepareShimInstall(shimPath string, target string, managedTargets []string, force bool, stdout io.Writer) (bool, error) {
	installed, err := isEnvoyageShimAny(shimPath, managedTargets)
	if err != nil {
		return false, err
	}
	if installed {
		if force {
			if err := os.Remove(shimPath); err != nil {
				return false, fmt.Errorf("remove existing shim %s: %w", shimPath, err)
			}
		} else {
			fmt.Fprintf(stdout, "shim already installed: %s -> %s\n", shimPath, target)
			printShimActivation(stdout, filepath.Dir(shimPath), "")
			return false, nil
		}
		return true, nil
	}

	if _, err := os.Lstat(shimPath); err == nil {
		return false, fmt.Errorf("refusing to overwrite non-Envoyage docker at %s", shimPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect shim path %s: %w", shimPath, err)
	}

	return true, nil
}

func runShimUninstall(args []string, stdout io.Writer) error {
	var binDir string
	var system bool

	flags := flag.NewFlagSet("shim uninstall", flag.ContinueOnError)
	flags.StringVar(&binDir, shimBinDirFlag, defaultShimBinDir, shimBinDirUsage)
	flags.BoolVar(&system, "system", false, "remove the docker shim from /usr/local/bin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("shim uninstall does not accept arguments")
	}

	binDirProvided := flagProvided(flags, shimBinDirFlag)
	if !system && !binDirProvided {
		if err := uninstallDefaultShimPaths(stdout); err != nil {
			return err
		}
		printShellHashRefresh(stdout)
		return nil
	}

	shimPath, err := dockerShimPathForMode(binDir, system, binDirProvided)
	if err != nil {
		return err
	}
	installPaths, err := shimEnvoyageInstallPaths(system)
	if err != nil {
		return err
	}
	if err := uninstallShimPath(shimPath, shimManagedTargets(installPaths.Target), stdout); err != nil {
		return err
	}
	printShellHashRefresh(stdout)
	return nil
}

func uninstallDefaultShimPaths(stdout io.Writer) error {
	userShimPath, err := dockerShimPath(defaultShimBinDir)
	if err != nil {
		return err
	}
	systemShimPath, err := dockerShimPath(defaultSystemShimBinDir)
	if err != nil {
		return err
	}
	userInstallPaths, err := shimEnvoyageInstallPaths(false)
	if err != nil {
		return err
	}
	systemInstallPaths, err := shimEnvoyageInstallPaths(true)
	if err != nil {
		return err
	}
	for _, candidate := range []struct {
		shimPath string
		targets  []string
	}{
		{shimPath: userShimPath, targets: shimManagedTargets(userInstallPaths.Target)},
		{shimPath: systemShimPath, targets: shimManagedTargets(systemInstallPaths.Target)},
	} {
		if err := uninstallShimPath(candidate.shimPath, candidate.targets, stdout); err != nil {
			return err
		}
	}
	return nil
}

func uninstallShimPath(shimPath string, managedTargets []string, stdout io.Writer) error {
	installed, err := isEnvoyageShimAny(shimPath, managedTargets)
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

func uninstallShimPathIfManaged(shimPath string, managedTargets []string, stdout io.Writer) error {
	installed, err := isEnvoyageShimAny(shimPath, managedTargets)
	if err != nil {
		return err
	}
	if !installed {
		if _, statErr := os.Lstat(shimPath); errors.Is(statErr, os.ErrNotExist) {
			fmt.Fprintf(stdout, "shim not installed: %s\n", shimPath)
		} else {
			fmt.Fprintf(stdout, "shim not managed by Envoyage: %s\n", shimPath)
		}
		return nil
	}
	if err := os.Remove(shimPath); err != nil {
		return fmt.Errorf("remove shim %s: %w", shimPath, err)
	}
	fmt.Fprintf(stdout, "removed shim: %s\n", shimPath)
	return nil
}

func runShimStatus(args []string, stdout io.Writer) error {
	var binDir string
	var system bool

	flags := flag.NewFlagSet("shim status", flag.ContinueOnError)
	flags.StringVar(&binDir, shimBinDirFlag, defaultShimBinDir, shimBinDirUsage)
	flags.BoolVar(&system, "system", false, "show the docker shim status under /usr/local/bin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("shim status does not accept arguments")
	}

	shimPath, err := dockerShimPathForMode(binDir, system, flagProvided(flags, shimBinDirFlag))
	if err != nil {
		return err
	}
	installPaths, err := shimEnvoyageInstallPaths(system)
	if err != nil {
		return err
	}
	installed, err := isEnvoyageShimAny(shimPath, shimManagedTargets(installPaths.Target))
	if err != nil {
		return err
	}

	printShimStatus(stdout, shimPath, installPaths.Target, installed)
	printDockerResolution(stdout, shimPath)

	if installed {
		printShimActivation(stdout, filepath.Dir(shimPath), os.Getenv("ENVOYAGE_DOCKER_BIN"))
	}
	return nil
}

func printShimStatus(stdout io.Writer, shimPath string, target string, installed bool) {
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
}

func printDockerResolution(stdout io.Writer, shimPath string) {
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
}

func dockerShimPathForMode(binDir string, system bool, binDirProvided bool) (string, error) {
	if system {
		if binDirProvided {
			return "", fmt.Errorf("--system cannot be combined with --bin-dir")
		}
		binDir = defaultSystemShimBinDir
	}
	return dockerShimPath(binDir)
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

func isEnvoyageShimAny(path string, targets []string) (bool, error) {
	for _, target := range targets {
		installed, err := isEnvoyageShim(path, target)
		if err != nil {
			return false, err
		}
		if installed {
			return true, nil
		}
	}
	return false, nil
}

func shimManagedTargets(installedTarget string) []string {
	targets := []string{installedTarget}
	if currentTarget, err := envoyageExecutablePath(); err == nil && currentTarget != installedTarget {
		targets = append(targets, currentTarget)
	}
	return targets
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
	printHashCommand(stdout)
}

func printShimUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, `Usage:
  envoyage shim status [--system] [--bin-dir ~/.local/bin]
  envoyage shim install [--system] [--bin-dir ~/.local/bin] [--force]
  envoyage shim uninstall [--system|--bin-dir ~/.local/bin]`)
}
