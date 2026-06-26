package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	defaultInstallBinDir = "~/.local/bin"
	defaultInstallLibDir = "~/.local/lib/envoyage"
)

const (
	installBinDirFlag  = "bin-dir"
	installBinDirUsage = "directory where the envoyage command symlink is installed"
	installLibDirFlag  = "lib-dir"
	installLibDirUsage = "directory where the envoyage binary is installed"
)

type installPaths struct {
	BinDir string
	LibDir string
	Target string
	Link   string
}

func runInstall(args []string, stdout io.Writer) error {
	var binDir string
	var libDir string
	var force bool

	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.StringVar(&binDir, installBinDirFlag, defaultInstallBinDir, installBinDirUsage)
	flags.StringVar(&libDir, installLibDirFlag, defaultInstallLibDir, installLibDirUsage)
	flags.BoolVar(&force, "force", false, "overwrite the installed Envoyage binary and recreate the Envoyage symlink")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("install does not accept arguments")
	}

	paths, err := resolveInstallPaths(binDir, libDir)
	if err != nil {
		return err
	}
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
	if err := installEnvoyageBinary(source, paths.Target, force); err != nil {
		return err
	}
	if err := installEnvoyageSymlink(paths.Link, paths.Target, force, stdout); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "installed envoyage: %s\n", paths.Target)
	fmt.Fprintf(stdout, "command symlink: %s -> %s\n", paths.Link, paths.Target)
	printInstallActivation(stdout, paths.BinDir)
	return nil
}

func runUninstall(args []string, stdout io.Writer) error {
	var binDir string
	var libDir string

	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	flags.StringVar(&binDir, installBinDirFlag, defaultInstallBinDir, installBinDirUsage)
	flags.StringVar(&libDir, installLibDirFlag, defaultInstallLibDir, installLibDirUsage)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("uninstall does not accept arguments")
	}

	paths, err := resolveInstallPaths(binDir, libDir)
	if err != nil {
		return err
	}
	if err := uninstallEnvoyageSymlink(paths.Link, paths.Target, stdout); err != nil {
		return err
	}
	if err := uninstallEnvoyageBinary(paths.Target, stdout); err != nil {
		return err
	}
	return nil
}

func runStatus(args []string, stdout io.Writer) error {
	var binDir string
	var libDir string

	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.StringVar(&binDir, installBinDirFlag, defaultInstallBinDir, installBinDirUsage)
	flags.StringVar(&libDir, installLibDirFlag, defaultInstallLibDir, installLibDirUsage)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("status does not accept arguments")
	}

	paths, err := resolveInstallPaths(binDir, libDir)
	if err != nil {
		return err
	}
	source, err := envoyageExecutablePath()
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "current executable: %s\n", source)
	fmt.Fprintf(stdout, "install target: %s\n", paths.Target)
	fmt.Fprintf(stdout, "command symlink: %s\n", paths.Link)
	printInstallState(stdout, paths)
	if pathEnvoyage, err := exec.LookPath("envoyage"); err == nil {
		fmt.Fprintf(stdout, "PATH envoyage: %s\n", pathEnvoyage)
	} else {
		fmt.Fprintln(stdout, "PATH envoyage: not found")
	}
	printInstallActivation(stdout, paths.BinDir)
	return nil
}

func resolveInstallPaths(binDir string, libDir string) (installPaths, error) {
	if binDir == "" {
		return installPaths{}, fmt.Errorf("--bin-dir is required")
	}
	if libDir == "" {
		return installPaths{}, fmt.Errorf("--lib-dir is required")
	}

	resolvedBinDir, err := expandHomePath(binDir)
	if err != nil {
		return installPaths{}, err
	}
	resolvedLibDir, err := expandHomePath(libDir)
	if err != nil {
		return installPaths{}, err
	}
	return installPaths{
		BinDir: resolvedBinDir,
		LibDir: resolvedLibDir,
		Target: filepath.Join(resolvedLibDir, "envoyage"),
		Link:   filepath.Join(resolvedBinDir, "envoyage"),
	}, nil
}

func installEnvoyageBinary(source string, target string, force bool) error {
	if samePath(source, target) {
		return nil
	}

	info, err := os.Lstat(target)
	switch {
	case err == nil && info.IsDir():
		return fmt.Errorf("refusing to overwrite directory at install target %s", target)
	case err == nil && !force:
		return fmt.Errorf("install target already exists at %s; pass --force to replace it", target)
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect install target %s: %w", target, err)
	}

	return copyExecutableFile(source, target)
}

func installEnvoyageSymlink(link string, target string, force bool, stdout io.Writer) error {
	installed, err := isSymlinkTo(link, target)
	if err != nil {
		return err
	}
	if installed {
		if !force {
			fmt.Fprintf(stdout, "command symlink already installed: %s -> %s\n", link, target)
			return nil
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove existing command symlink %s: %w", link, err)
		}
	} else if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("refusing to overwrite non-Envoyage command at %s", link)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect command symlink %s: %w", link, err)
	}

	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("create command symlink %s -> %s: %w", link, target, err)
	}
	return nil
}

func uninstallEnvoyageSymlink(link string, target string, stdout io.Writer) error {
	installed, err := isSymlinkTo(link, target)
	if err != nil {
		return err
	}
	if installed {
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove command symlink %s: %w", link, err)
		}
		fmt.Fprintf(stdout, "removed command symlink: %s\n", link)
		return nil
	}

	if _, err := os.Lstat(link); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stdout, "command symlink not installed: %s\n", link)
		return nil
	}
	return fmt.Errorf("refusing to remove non-Envoyage command at %s", link)
}

func uninstallEnvoyageBinary(target string, stdout io.Writer) error {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stdout, "installed binary not found: %s\n", target)
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect installed binary %s: %w", target, err)
	}
	if info.IsDir() {
		return fmt.Errorf("refusing to remove directory at install target %s", target)
	}
	if err := os.Remove(target); err != nil {
		return fmt.Errorf("remove installed binary %s: %w", target, err)
	}
	fmt.Fprintf(stdout, "removed installed binary: %s\n", target)
	return nil
}

func copyExecutableFile(source string, target string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source executable %s: %w", source, err)
	}
	defer sourceFile.Close()

	tmpTarget := target + ".tmp"
	targetFile, err := os.OpenFile(tmpTarget, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create temporary install target %s: %w", tmpTarget, err)
	}
	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		targetFile.Close()
		os.Remove(tmpTarget)
		return fmt.Errorf("copy executable to %s: %w", tmpTarget, err)
	}
	if err := targetFile.Close(); err != nil {
		os.Remove(tmpTarget)
		return fmt.Errorf("close temporary install target %s: %w", tmpTarget, err)
	}
	if err := os.Chmod(tmpTarget, 0o755); err != nil {
		os.Remove(tmpTarget)
		return fmt.Errorf("set install target permissions %s: %w", tmpTarget, err)
	}
	if err := os.Rename(tmpTarget, target); err != nil {
		os.Remove(tmpTarget)
		return fmt.Errorf("replace install target %s: %w", target, err)
	}
	return nil
}

func isSymlinkTo(path string, target string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect symlink %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}

	linkTarget, err := os.Readlink(path)
	if err != nil {
		return false, fmt.Errorf("read symlink %s: %w", path, err)
	}
	resolved := linkTarget
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(path), resolved)
	}
	return samePath(resolved, target), nil
}

func printInstallState(stdout io.Writer, paths installPaths) {
	if _, err := os.Stat(paths.Target); err == nil {
		fmt.Fprintln(stdout, "installed binary: yes")
	} else {
		fmt.Fprintln(stdout, "installed binary: no")
	}

	if installed, err := isSymlinkTo(paths.Link, paths.Target); err == nil && installed {
		fmt.Fprintln(stdout, "command symlink installed: yes")
	} else {
		fmt.Fprintln(stdout, "command symlink installed: no")
	}
	if linkTarget, ok := readSymlinkTarget(paths.Link); ok {
		fmt.Fprintf(stdout, "command symlink target: %s\n", linkTarget)
	}
}

func printInstallActivation(stdout io.Writer, binDir string) {
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "To activate envoyage:")
	fmt.Fprintf(stdout, "  export PATH=%q:$PATH\n", binDir)
}
