package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	updateHTTPClient = &http.Client{Timeout: 60 * time.Second}
	updateAPIBaseURL = "https://api.github.com/repos/pletorco/envoyage"
)

type updateOptions struct {
	CheckOnly  bool
	Version    string
	BinDir     string
	LibDir     string
	System     bool
	HTTPClient *http.Client
	APIBaseURL string
	GOOS       string
	GOARCH     string
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

func runUpdate(args []string, stdout io.Writer) error {
	opts := updateOptions{
		BinDir:     defaultInstallBinDir,
		LibDir:     defaultInstallLibDir,
		HTTPClient: updateHTTPClient,
		APIBaseURL: updateAPIBaseURL,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
	}

	flags := flagSet("update")
	flags.BoolVar(&opts.CheckOnly, "check", false, "check for an available update without installing it")
	flags.StringVar(&opts.Version, "version", "", "release version to install, for example 0.4.0")
	flags.StringVar(&opts.BinDir, installBinDirFlag, defaultInstallBinDir, installBinDirUsage)
	flags.StringVar(&opts.LibDir, installLibDirFlag, defaultInstallLibDir, installLibDirUsage)
	flags.BoolVar(&opts.System, "system", false, installSystemUsage)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("update does not accept arguments")
	}

	opts.Version = strings.TrimPrefix(opts.Version, "v")
	if opts.Version != "" && !validVersionString(opts.Version) {
		return fmt.Errorf("invalid --version %q\n\nhint:\n  use semantic version format, for example: envoyage update --version 0.4.0", opts.Version)
	}

	return updateEnvoyage(context.Background(), opts, flagProvided(flags, installBinDirFlag), flagProvided(flags, installLibDirFlag), stdout)
}

func flagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}

func updateEnvoyage(ctx context.Context, opts updateOptions, binDirProvided bool, libDirProvided bool, stdout io.Writer) error {
	client, apiBaseURL := updateClientAndAPI(opts)

	release, err := fetchRelease(ctx, client, apiBaseURL, opts.Version)
	if err != nil {
		return err
	}
	targetVersion, err := updateTargetVersion(release, opts.Version)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "current: %s\n", version)
	fmt.Fprintf(stdout, "latest: %s\n", targetVersion)
	if updateHandledWithoutInstall(opts, targetVersion, stdout) {
		return nil
	}

	paths, err := resolveInstallPathsForMode(opts.BinDir, opts.LibDir, opts.System, binDirProvided, libDirProvided)
	if err != nil {
		return err
	}
	if err := requireInstalledTarget(paths.Target); err != nil {
		return err
	}

	return installUpdateRelease(ctx, client, release, targetVersion, opts, paths, stdout)
}

func updateClientAndAPI(opts updateOptions) (*http.Client, string) {
	client := opts.HTTPClient
	if client == nil {
		client = updateHTTPClient
	}
	apiBaseURL := strings.TrimRight(opts.APIBaseURL, "/")
	if apiBaseURL == "" {
		apiBaseURL = updateAPIBaseURL
	}
	return client, apiBaseURL
}

func updateTargetVersion(release githubRelease, requestedVersion string) (string, error) {
	targetVersion := strings.TrimPrefix(release.TagName, "v")
	if requestedVersion != "" {
		targetVersion = requestedVersion
	}
	if targetVersion == "" {
		return "", fmt.Errorf("release response did not include a version")
	}
	return targetVersion, nil
}

func updateHandledWithoutInstall(opts updateOptions, targetVersion string, stdout io.Writer) bool {
	comparison := compareVersions(version, targetVersion)
	if opts.CheckOnly {
		if comparison >= 0 {
			fmt.Fprintln(stdout, "up-to-date: yes")
		} else {
			fmt.Fprintln(stdout, "up-to-date: no")
		}
		return true
	}
	if opts.Version == "" && comparison >= 0 {
		fmt.Fprintln(stdout, "envoyage is already up to date")
		return true
	}
	return false
}

func installUpdateRelease(ctx context.Context, client *http.Client, release githubRelease, targetVersion string, opts updateOptions, paths installPaths, stdout io.Writer) error {
	archiveName := releaseArchiveName(targetVersion, opts.GOOS, opts.GOARCH)
	archiveAsset, checksumAsset, err := updateReleaseAssets(release, archiveName, opts)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "envoyage-update-")
	if err != nil {
		return fmt.Errorf("create update temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(ctx, client, archiveAsset.DownloadURL, archivePath); err != nil {
		return err
	}
	if err := downloadFile(ctx, client, checksumAsset.DownloadURL, checksumPath); err != nil {
		return err
	}
	if err := verifyChecksum(archivePath, checksumPath, archiveName); err != nil {
		return err
	}

	extractedPath, err := extractReleaseBinary(archivePath, archiveName, tmpDir)
	if err != nil {
		return err
	}
	if err := replaceInstalledBinary(extractedPath, paths.Target); err != nil {
		return err
	}
	if err := installEnvoyageSymlink(paths.Link, paths.Target, true, stdout); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "updated envoyage: %s\n", paths.Target)
	fmt.Fprintf(stdout, "version: %s\n", targetVersion)
	printShellHashRefresh(stdout)
	return nil
}

func updateReleaseAssets(release githubRelease, archiveName string, opts updateOptions) (githubReleaseAsset, githubReleaseAsset, error) {
	archiveAsset, ok := findReleaseAsset(release.Assets, archiveName)
	if !ok {
		return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("release asset %s not found\n\nhint:\n  check that Envoyage publishes builds for %s/%s or choose another version with --version", archiveName, opts.GOOS, opts.GOARCH)
	}
	checksumAsset, ok := findReleaseAsset(release.Assets, "checksums.txt")
	if !ok {
		return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("release checksums.txt not found\n\nhint:\n  refuse to update because the release cannot be verified")
	}
	return archiveAsset, checksumAsset, nil
}

func fetchRelease(ctx context.Context, client *http.Client, apiBaseURL string, version string) (githubRelease, error) {
	endpoint := apiBaseURL + "/releases/latest"
	if version != "" {
		endpoint = apiBaseURL + "/releases/tags/v" + version
	}
	var release githubRelease
	if err := fetchJSON(ctx, client, endpoint, &release); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func fetchJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "envoyage/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch release metadata: %w\n\nhint:\n  check network access to GitHub or retry with --version once the release exists", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("fetch release metadata: %s\n\nhint:\n  check that the release exists on GitHub", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("parse release metadata: %w", err)
	}
	return nil
}

func downloadFile(ctx context.Context, client *http.Client, url string, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create download request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "envoyage/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download release asset %s: %w", filepath.Base(path), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("download release asset %s: %s", filepath.Base(path), resp.Status)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create downloaded asset %s: %w", path, err)
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("write downloaded asset %s: %w", path, err)
	}
	return nil
}

func verifyChecksum(archivePath string, checksumPath string, archiveName string) error {
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read checksums file: %w", err)
	}
	want := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archiveName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksum for %s not found\n\nhint:\n  refuse to update because the release asset cannot be verified", archiveName)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open release asset for checksum: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash release asset: %w", err)
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s\n\nhint:\n  downloaded asset did not match checksums.txt; retry later and do not run the downloaded file", archiveName)
	}
	return nil
}

func extractReleaseBinary(archivePath string, archiveName string, tmpDir string) (string, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return extractZipBinary(archivePath, tmpDir)
	}
	return extractTarGzBinary(archivePath, tmpDir)
}

func extractTarGzBinary(archivePath string, tmpDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open release archive: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("read release archive gzip: %w", err)
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read release archive: %w", err)
		}
		if header.FileInfo().IsDir() {
			continue
		}
		return writeExtractedBinary(reader, filepath.Join(tmpDir, filepath.Base(header.Name)))
	}
	return "", fmt.Errorf("release archive did not contain an Envoyage binary")
}

func extractZipBinary(archivePath string, tmpDir string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("open release zip: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("open zipped binary %s: %w", file.Name, err)
		}
		defer src.Close()
		return writeExtractedBinary(src, filepath.Join(tmpDir, filepath.Base(file.Name)))
	}
	return "", fmt.Errorf("release zip did not contain an Envoyage binary")
}

func writeExtractedBinary(src io.Reader, path string) (string, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return "", fmt.Errorf("create extracted binary %s: %w", path, err)
	}
	if _, err := io.Copy(file, src); err != nil {
		file.Close()
		return "", fmt.Errorf("write extracted binary %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close extracted binary %s: %w", path, err)
	}
	return path, nil
}

func replaceInstalledBinary(source string, target string) error {
	tmpTarget := target + ".update"
	if err := copyExecutableFile(source, tmpTarget); err != nil {
		return err
	}
	if err := os.Rename(tmpTarget, target); err != nil {
		os.Remove(tmpTarget)
		return fmt.Errorf("replace installed binary %s: %w\n\nhint:\n  close running Envoyage processes and retry\n  for system installs, run: sudo envoyage update --system", target, err)
	}
	return nil
}

func requireInstalledTarget(target string) error {
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("installed Envoyage binary not found at %s: %w\n\nhint:\n  install Envoyage first with: envoyage install\n  or update a system install with: sudo envoyage update --system", target, err)
	}
	if info.IsDir() {
		return fmt.Errorf("installed Envoyage target is a directory at %s", target)
	}
	return nil
}

func releaseArchiveName(version string, goos string, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("envoyage_%s_%s_%s%s", version, goos, goarch, ext)
}

func findReleaseAsset(assets []githubReleaseAsset, name string) (githubReleaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return githubReleaseAsset{}, false
}

func validVersionString(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func compareVersions(left string, right string) int {
	left = strings.TrimPrefix(left, "v")
	right = strings.TrimPrefix(right, "v")
	if !validVersionString(left) || !validVersionString(right) {
		return strings.Compare(left, right)
	}
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for i := range leftParts {
		l := atoi(leftParts[i])
		r := atoi(rightParts[i])
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	}
	return 0
}

func atoi(value string) int {
	n := 0
	for _, r := range value {
		n = n*10 + int(r-'0')
	}
	return n
}
