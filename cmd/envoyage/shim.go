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
	shimBinDirFlag    = "bin-dir"
	shimBinDirUsage   = "directory where runtime shim symlinks are installed"
	shimRuntimeFlag   = "runtime"
	shimRuntimeUsage  = "runtime shim to manage: auto, docker, podman, or all"
	shimRuntimeAuto   = "auto"
	shimRuntimeAll    = "all"
	shimRuntimeDocker = "docker"
	shimRuntimePodman = "podman"
)

var supportedShimRuntimes = []string{shimRuntimeDocker, shimRuntimePodman}

type shimInstallRequest struct {
	BinDir         string
	System         bool
	BinDirProvided bool
	Target         string
	ManagedTargets []string
	Force          bool
	Stdout         io.Writer
}

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
	var runtimeName string

	flags := flag.NewFlagSet("shim install", flag.ContinueOnError)
	flags.StringVar(&binDir, shimBinDirFlag, defaultShimBinDir, shimBinDirUsage)
	flags.StringVar(&runtimeName, shimRuntimeFlag, shimRuntimeAuto, shimRuntimeUsage)
	flags.BoolVar(&force, "force", false, "recreate an existing Envoyage-managed shim symlink")
	flags.BoolVar(&system, "system", false, "install runtime shims to /usr/local/bin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("shim install does not accept arguments")
	}

	binDirProvided := flagProvided(flags, shimBinDirFlag)
	runtimes, err := shimInstallRuntimes(runtimeName, binDir, system, binDirProvided)
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

	installedAny, err := installRuntimeShims(runtimes, binDir, system, binDirProvided, installPaths.Target, force, stdout)
	if err != nil {
		return err
	}
	if installedAny {
		printShimActivation(stdout, shimBinDirForMode(binDir, system), runtimes)
	}
	return nil
}

func installRuntimeShims(runtimes []string, binDir string, system bool, binDirProvided bool, target string, force bool, stdout io.Writer) (bool, error) {
	installedAny := false
	request := shimInstallRequest{
		BinDir:         binDir,
		System:         system,
		BinDirProvided: binDirProvided,
		Target:         target,
		ManagedTargets: shimManagedTargets(target),
		Force:          force,
		Stdout:         stdout,
	}
	for _, runtimeName := range runtimes {
		installed, err := installRuntimeShim(runtimeName, request)
		if err != nil {
			return false, err
		}
		if installed {
			installedAny = true
		}
	}
	return installedAny, nil
}

func installRuntimeShim(runtimeName string, request shimInstallRequest) (bool, error) {
	shimPath, err := runtimeShimPathForMode(runtimeName, request.BinDir, request.System, request.BinDirProvided)
	if err != nil {
		return false, err
	}
	if err := mkdirAll(filepath.Dir(shimPath), 0o755); err != nil {
		return false, fmt.Errorf("create shim directory %s: %w", filepath.Dir(shimPath), err)
	}

	shouldInstall, err := prepareShimInstall(runtimeName, shimPath, request.Target, request.ManagedTargets, request.Force, request.Stdout)
	if err != nil {
		return false, err
	}
	if !shouldInstall {
		return false, nil
	}
	if err := os.Symlink(request.Target, shimPath); err != nil {
		return false, fmt.Errorf("create %s shim %s -> %s: %w", runtimeName, shimPath, request.Target, err)
	}
	fmt.Fprintf(request.Stdout, "installed %s shim: %s -> %s\n", runtimeName, shimPath, request.Target)
	return true, nil
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

func prepareShimInstall(runtimeName string, shimPath string, target string, managedTargets []string, force bool, stdout io.Writer) (bool, error) {
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
			printShimActivation(stdout, filepath.Dir(shimPath), []string{runtimeName})
			return false, nil
		}
		return true, nil
	}

	if _, err := os.Lstat(shimPath); err == nil {
		return false, fmt.Errorf("refusing to overwrite non-Envoyage %s at %s", runtimeName, shimPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect shim path %s: %w", shimPath, err)
	}

	return true, nil
}

func runShimUninstall(args []string, stdout io.Writer) error {
	var binDir string
	var system bool
	var runtimeName string

	flags := flag.NewFlagSet("shim uninstall", flag.ContinueOnError)
	flags.StringVar(&binDir, shimBinDirFlag, defaultShimBinDir, shimBinDirUsage)
	flags.StringVar(&runtimeName, shimRuntimeFlag, shimRuntimeAuto, shimRuntimeUsage)
	flags.BoolVar(&system, "system", false, "remove runtime shims from /usr/local/bin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("shim uninstall does not accept arguments")
	}

	binDirProvided := flagProvided(flags, shimBinDirFlag)
	runtimes, err := shimInspectionRuntimes(runtimeName)
	if err != nil {
		return err
	}
	if !system && !binDirProvided {
		if err := uninstallDefaultShimPaths(runtimes, stdout); err != nil {
			return err
		}
		printShellHashRefresh(stdout)
		return nil
	}

	installPaths, err := shimEnvoyageInstallPaths(system)
	if err != nil {
		return err
	}
	for _, runtimeName := range runtimes {
		shimPath, err := runtimeShimPathForMode(runtimeName, binDir, system, binDirProvided)
		if err != nil {
			return err
		}
		if err := uninstallShimPath(runtimeName, shimPath, shimManagedTargets(installPaths.Target), stdout); err != nil {
			return err
		}
	}
	printShellHashRefresh(stdout)
	return nil
}

func uninstallDefaultShimPaths(runtimes []string, stdout io.Writer) error {
	userInstallPaths, err := shimEnvoyageInstallPaths(false)
	if err != nil {
		return err
	}
	systemInstallPaths, err := shimEnvoyageInstallPaths(true)
	if err != nil {
		return err
	}

	for _, runtimeName := range runtimes {
		userShimPath, err := runtimeShimPath(runtimeName, defaultShimBinDir)
		if err != nil {
			return err
		}
		systemShimPath, err := runtimeShimPath(runtimeName, defaultSystemShimBinDir)
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
			if err := uninstallShimPath(runtimeName, candidate.shimPath, candidate.targets, stdout); err != nil {
				return err
			}
		}
	}
	return nil
}

func uninstallShimPath(runtimeName string, shimPath string, managedTargets []string, stdout io.Writer) error {
	installed, err := isEnvoyageShimAny(shimPath, managedTargets)
	if err != nil {
		return err
	}
	if !installed {
		if _, statErr := os.Lstat(shimPath); errors.Is(statErr, os.ErrNotExist) {
			fmt.Fprintf(stdout, "shim not installed: %s\n", shimPath)
			return nil
		}
		return fmt.Errorf("refusing to remove non-Envoyage %s at %s", runtimeName, shimPath)
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
	var runtimeName string

	flags := flag.NewFlagSet("shim status", flag.ContinueOnError)
	flags.StringVar(&binDir, shimBinDirFlag, defaultShimBinDir, shimBinDirUsage)
	flags.StringVar(&runtimeName, shimRuntimeFlag, shimRuntimeAuto, shimRuntimeUsage)
	flags.BoolVar(&system, "system", false, "show runtime shim status under /usr/local/bin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("shim status does not accept arguments")
	}

	installPaths, err := shimEnvoyageInstallPaths(system)
	if err != nil {
		return err
	}
	runtimes, err := shimInspectionRuntimes(runtimeName)
	if err != nil {
		return err
	}

	for i, runtimeName := range runtimes {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		shimPath, err := runtimeShimPathForMode(runtimeName, binDir, system, flagProvided(flags, shimBinDirFlag))
		if err != nil {
			return err
		}
		installed, err := isEnvoyageShimAny(shimPath, shimManagedTargets(installPaths.Target))
		if err != nil {
			return err
		}

		printShimStatus(stdout, runtimeName, shimPath, installPaths.Target, installed)
		printRuntimeResolution(stdout, runtimeName, shimPath)
		if installed {
			printShimActivation(stdout, filepath.Dir(shimPath), []string{runtimeName})
		}
	}
	return nil
}

func printShimStatus(stdout io.Writer, runtimeName string, shimPath string, target string, installed bool) {
	fmt.Fprintf(stdout, "runtime: %s\n", runtimeName)
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
	printRuntimeResolution(stdout, shimRuntimeDocker, shimPath)
}

func printRuntimeResolution(stdout io.Writer, runtimeName string, shimPath string) {
	if pathRuntime, err := exec.LookPath(runtimeName); err == nil {
		fmt.Fprintf(stdout, "PATH %s: %s\n", runtimeName, pathRuntime)
	} else {
		fmt.Fprintf(stdout, "PATH %s: not found\n", runtimeName)
	}

	envName := compose.RuntimeBinEnv(runtimeName)
	runtimeBin := os.Getenv(envName)
	if runtimeBin == "" {
		fmt.Fprintf(stdout, "%s: not set\n", envName)
		if realRuntime, err := compose.FindRealRuntimeBin(runtimeName, shimPath); err == nil {
			fmt.Fprintf(stdout, "real %s candidate: %s\n", runtimeName, realRuntime)
		} else {
			fmt.Fprintf(stdout, "real %s candidate: %v\n", runtimeName, err)
		}
	} else {
		fmt.Fprintf(stdout, "%s: %s\n", envName, runtimeBin)
	}
}

func dockerShimPathForMode(binDir string, system bool, binDirProvided bool) (string, error) {
	return runtimeShimPathForMode(shimRuntimeDocker, binDir, system, binDirProvided)
}

func runtimeShimPathForMode(runtimeName string, binDir string, system bool, binDirProvided bool) (string, error) {
	if system {
		if binDirProvided {
			return "", fmt.Errorf("--system cannot be combined with --bin-dir")
		}
		binDir = defaultSystemShimBinDir
	}
	return runtimeShimPath(runtimeName, binDir)
}

func dockerShimPath(binDir string) (string, error) {
	return runtimeShimPath(shimRuntimeDocker, binDir)
}

func runtimeShimPath(runtimeName string, binDir string) (string, error) {
	dir, err := expandHomePath(binDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, runtimeName), nil
}

func shimBinDirForMode(binDir string, system bool) string {
	if system {
		return defaultSystemShimBinDir
	}
	dir, err := expandHomePath(binDir)
	if err != nil {
		return binDir
	}
	return dir
}

func shimInstallRuntimes(runtimeName string, binDir string, system bool, binDirProvided bool) ([]string, error) {
	switch runtimeName {
	case shimRuntimeDocker, shimRuntimePodman:
		return []string{runtimeName}, nil
	case shimRuntimeAll:
		return append([]string(nil), supportedShimRuntimes...), nil
	case shimRuntimeAuto:
		var detected []string
		for _, candidate := range supportedShimRuntimes {
			shimPath, err := runtimeShimPathForMode(candidate, binDir, system, binDirProvided)
			if err != nil {
				return nil, err
			}
			if shimRuntimeAvailable(candidate, shimPath) {
				detected = append(detected, candidate)
			}
		}
		if len(detected) == 0 {
			return nil, fmt.Errorf("no supported runtime found for shim install; install docker or podman, or pass --runtime docker|podman|all")
		}
		return detected, nil
	default:
		return nil, fmt.Errorf("unsupported shim runtime %q; use auto, docker, podman, or all", runtimeName)
	}
}

func shimInspectionRuntimes(runtimeName string) ([]string, error) {
	switch runtimeName {
	case shimRuntimeDocker, shimRuntimePodman:
		return []string{runtimeName}, nil
	case shimRuntimeAuto, shimRuntimeAll:
		return append([]string(nil), supportedShimRuntimes...), nil
	default:
		return nil, fmt.Errorf("unsupported shim runtime %q; use auto, docker, podman, or all", runtimeName)
	}
}

func shimRuntimeAvailable(runtimeName string, shimPath string) bool {
	if os.Getenv(compose.RuntimeBinEnv(runtimeName)) != "" {
		return true
	}
	_, err := compose.FindRealRuntimeBin(runtimeName, shimPath)
	return err == nil
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

func printShimActivation(stdout io.Writer, binDir string, runtimes []string) {
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "To activate shim mode:")
	fmt.Fprintf(stdout, "  export PATH=%q:$PATH\n", binDir)
	for _, runtimeName := range runtimes {
		envName := compose.RuntimeBinEnv(runtimeName)
		if os.Getenv(envName) == "" {
			fmt.Fprintf(stdout, "  export %s=/usr/bin/%s\n", envName, runtimeName)
		}
	}
	printHashCommand(stdout)
}

func printShimUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, `Usage:
  envoyage shim status [--runtime auto|docker|podman|all] [--system] [--bin-dir ~/.local/bin]
  envoyage shim install [--runtime auto|docker|podman|all] [--system] [--bin-dir ~/.local/bin] [--force]
  envoyage shim uninstall [--runtime auto|docker|podman|all] [--system|--bin-dir ~/.local/bin]`)
}
