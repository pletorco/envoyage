package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateCheckReportsLatest(t *testing.T) {
	server := newUpdateTestServer(t, "0.5.1", releaseArchiveName("0.5.1", "linux", "amd64"), nil, nil)
	defer server.Close()

	var out bytes.Buffer
	err := updateEnvoyage(context.Background(), updateOptions{
		CheckOnly:  true,
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
	}, false, false, &out)
	if err != nil {
		t.Fatalf("updateEnvoyage(check) error = %v", err)
	}

	output := out.String()
	for _, want := range []string{"current: 0.5.0", "latest: 0.5.1", "up-to-date: no"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestUpdateInstallsVerifiedTarball(t *testing.T) {
	archiveName := releaseArchiveName("0.4.1", "linux", "amd64")
	archiveBytes := makeUpdateTarGz(t, "envoyage", []byte("new-binary\n"))
	checksumBytes := []byte(fmt.Sprintf("%s  %s\n", sha256String(archiveBytes), archiveName))
	server := newUpdateTestServer(t, "0.4.1", archiveName, archiveBytes, checksumBytes)
	defer server.Close()

	paths := prepareInstalledUpdateTarget(t, "old-binary\n")
	var out bytes.Buffer
	err := updateEnvoyage(context.Background(), updateOptions{
		Version:    "0.4.1",
		BinDir:     paths.BinDir,
		LibDir:     paths.LibDir,
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
	}, true, true, &out)
	if err != nil {
		t.Fatalf("updateEnvoyage() error = %v", err)
	}

	data, err := os.ReadFile(paths.Target)
	if err != nil {
		t.Fatalf("read updated target: %v", err)
	}
	if string(data) != "new-binary\n" {
		t.Fatalf("updated target = %q, want new binary", string(data))
	}
	if ok, err := isSymlinkTo(paths.Link, paths.Target); err != nil || !ok {
		t.Fatalf("command symlink installed = %v, err = %v", ok, err)
	}
	if !strings.Contains(out.String(), "updated envoyage: "+paths.Target) {
		t.Fatalf("output missing update confirmation:\n%s", out.String())
	}
}

func TestUpdateInstallsVerifiedZip(t *testing.T) {
	archiveName := releaseArchiveName("0.4.1", "windows", "amd64")
	archiveBytes := makeUpdateZip(t, "envoyage.exe", []byte("new-windows-binary\n"))
	checksumBytes := []byte(fmt.Sprintf("%s  %s\n", sha256String(archiveBytes), archiveName))
	server := newUpdateTestServer(t, "0.4.1", archiveName, archiveBytes, checksumBytes)
	defer server.Close()

	paths := prepareInstalledUpdateTarget(t, "old-binary\n")
	err := updateEnvoyage(context.Background(), updateOptions{
		Version:    "0.4.1",
		BinDir:     paths.BinDir,
		LibDir:     paths.LibDir,
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "windows",
		GOARCH:     "amd64",
	}, true, true, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("updateEnvoyage(zip) error = %v", err)
	}

	data, readErr := os.ReadFile(paths.Target)
	if readErr != nil {
		t.Fatalf("read updated target: %v", readErr)
	}
	if string(data) != "new-windows-binary\n" {
		t.Fatalf("updated target = %q, want zip binary", string(data))
	}
}

func TestUpdateSkipsWhenAlreadyCurrent(t *testing.T) {
	server := newUpdateTestServer(t, version, releaseArchiveName(version, "linux", "amd64"), nil, nil)
	defer server.Close()

	var out bytes.Buffer
	err := updateEnvoyage(context.Background(), updateOptions{
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
	}, false, false, &out)
	if err != nil {
		t.Fatalf("updateEnvoyage(current) error = %v", err)
	}
	if !strings.Contains(out.String(), "envoyage is already up to date") {
		t.Fatalf("output = %q, want up-to-date message", out.String())
	}
}

func TestUpdateRejectsChecksumMismatch(t *testing.T) {
	archiveName := releaseArchiveName("0.4.1", "linux", "amd64")
	archiveBytes := makeUpdateTarGz(t, "envoyage", []byte("new-binary\n"))
	checksumBytes := []byte(strings.Repeat("0", 64) + "  " + archiveName + "\n")
	server := newUpdateTestServer(t, "0.4.1", archiveName, archiveBytes, checksumBytes)
	defer server.Close()

	paths := prepareInstalledUpdateTarget(t, "old-binary\n")
	err := updateEnvoyage(context.Background(), updateOptions{
		Version:    "0.4.1",
		BinDir:     paths.BinDir,
		LibDir:     paths.LibDir,
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
	}, true, true, &bytes.Buffer{})
	if err == nil {
		t.Fatal("updateEnvoyage() error = nil, want checksum error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %q, want checksum mismatch", err.Error())
	}
	data, readErr := os.ReadFile(paths.Target)
	if readErr != nil {
		t.Fatalf("read target after failed update: %v", readErr)
	}
	if string(data) != "old-binary\n" {
		t.Fatalf("target changed after failed update: %q", string(data))
	}
}

func TestUpdateRejectsMissingArchiveAsset(t *testing.T) {
	server := newUpdateTestServer(t, "0.4.1", "envoyage_0.4.1_linux_amd64.tar.gz", nil, nil)
	defer server.Close()

	paths := prepareInstalledUpdateTarget(t, "old-binary\n")
	err := updateEnvoyage(context.Background(), updateOptions{
		Version:    "0.4.1",
		BinDir:     paths.BinDir,
		LibDir:     paths.LibDir,
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "darwin",
		GOARCH:     "arm64",
	}, true, true, &bytes.Buffer{})
	if err == nil {
		t.Fatal("updateEnvoyage() error = nil, want missing asset error")
	}
	if !strings.Contains(err.Error(), "release asset envoyage_0.4.1_darwin_arm64.tar.gz not found") {
		t.Fatalf("error = %q, want missing archive asset", err.Error())
	}
}

func TestUpdateRejectsMissingChecksumEntry(t *testing.T) {
	archiveName := releaseArchiveName("0.4.1", "linux", "amd64")
	archiveBytes := makeUpdateTarGz(t, "envoyage", []byte("new-binary\n"))
	server := newUpdateTestServer(t, "0.4.1", archiveName, archiveBytes, []byte(strings.Repeat("1", 64)+"  other.tar.gz\n"))
	defer server.Close()

	paths := prepareInstalledUpdateTarget(t, "old-binary\n")
	err := updateEnvoyage(context.Background(), updateOptions{
		Version:    "0.4.1",
		BinDir:     paths.BinDir,
		LibDir:     paths.LibDir,
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
	}, true, true, &bytes.Buffer{})
	if err == nil {
		t.Fatal("updateEnvoyage() error = nil, want missing checksum error")
	}
	if !strings.Contains(err.Error(), "checksum for "+archiveName+" not found") {
		t.Fatalf("error = %q, want missing checksum entry", err.Error())
	}
}

func TestUpdateRejectsMissingChecksumAsset(t *testing.T) {
	archiveName := releaseArchiveName("0.4.1", "linux", "amd64")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/tags/v0.4.1" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":"v0.4.1","assets":[{"name":%q,"browser_download_url":"http://%s/assets/archive"}]}`, archiveName, r.Host)
	}))
	defer server.Close()

	paths := prepareInstalledUpdateTarget(t, "old-binary\n")
	err := updateEnvoyage(context.Background(), updateOptions{
		Version:    "0.4.1",
		BinDir:     paths.BinDir,
		LibDir:     paths.LibDir,
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
	}, true, true, &bytes.Buffer{})
	if err == nil {
		t.Fatal("updateEnvoyage() error = nil, want missing checksums asset error")
	}
	if !strings.Contains(err.Error(), "release checksums.txt not found") {
		t.Fatalf("error = %q, want missing checksums asset", err.Error())
	}
}

func TestUpdateRejectsDownloadFailure(t *testing.T) {
	archiveName := releaseArchiveName("0.4.1", "linux", "amd64")
	server := newUpdateTestServer(t, "0.4.1", archiveName, nil, []byte("unused"))
	defer server.Close()

	paths := prepareInstalledUpdateTarget(t, "old-binary\n")
	err := updateEnvoyage(context.Background(), updateOptions{
		Version:    "0.4.1",
		BinDir:     paths.BinDir,
		LibDir:     paths.LibDir,
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
	}, true, true, &bytes.Buffer{})
	if err == nil {
		t.Fatal("updateEnvoyage() error = nil, want download failure")
	}
	if !strings.Contains(err.Error(), "download release asset "+archiveName+": 404") {
		t.Fatalf("error = %q, want download failure", err.Error())
	}
}

func TestUpdateRequiresInstalledTarget(t *testing.T) {
	server := newUpdateTestServer(t, "0.4.1", releaseArchiveName("0.4.1", "linux", "amd64"), nil, nil)
	defer server.Close()

	tmpDir := t.TempDir()
	err := updateEnvoyage(context.Background(), updateOptions{
		Version:    "0.4.1",
		BinDir:     filepath.Join(tmpDir, "bin"),
		LibDir:     filepath.Join(tmpDir, "lib"),
		HTTPClient: server.Client(),
		APIBaseURL: server.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
	}, true, true, &bytes.Buffer{})
	if err == nil {
		t.Fatal("updateEnvoyage() error = nil, want missing install error")
	}
	for _, want := range []string{"installed Envoyage binary not found", "envoyage install"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
}

func TestFetchReleaseRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	_, err := fetchRelease(context.Background(), server.Client(), server.URL, "9.9.9")
	if err == nil {
		t.Fatal("fetchRelease() error = nil, want HTTP error")
	}
	if !strings.Contains(err.Error(), "fetch release metadata: 404") {
		t.Fatalf("error = %q, want HTTP 404", err.Error())
	}
}

func TestRunUpdateRejectsInvalidVersion(t *testing.T) {
	err := runUpdate([]string{"--version", "latest"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runUpdate() error = nil, want invalid version error")
	}
	if !strings.Contains(err.Error(), "invalid --version") {
		t.Fatalf("error = %q, want invalid --version", err.Error())
	}
}

func TestRunUpdateRejectsUnexpectedArgs(t *testing.T) {
	err := runUpdate([]string{"extra"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runUpdate() error = nil, want unexpected argument error")
	}
	if !strings.Contains(err.Error(), "update does not accept arguments") {
		t.Fatalf("error = %q, want unexpected argument error", err.Error())
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{name: "less", left: "0.4.0", right: "0.4.1", want: -1},
		{name: "equal with v", left: "v0.4.1", right: "0.4.1", want: 0},
		{name: "greater", left: "0.5.0", right: "0.4.9", want: 1},
		{name: "fallback", left: "dev", right: "0.4.1", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.left, tt.right)
			if got != tt.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestReleaseArchiveName(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		goarch string
		want   string
	}{
		{name: "linux", goos: "linux", goarch: "amd64", want: "envoyage_0.4.1_linux_amd64.tar.gz"},
		{name: "darwin", goos: "darwin", goarch: "arm64", want: "envoyage_0.4.1_darwin_arm64.tar.gz"},
		{name: "windows", goos: "windows", goarch: "amd64", want: "envoyage_0.4.1_windows_amd64.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := releaseArchiveName("0.4.1", tt.goos, tt.goarch)
			if got != tt.want {
				t.Fatalf("releaseArchiveName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func newUpdateTestServer(t *testing.T, releaseVersion string, archiveName string, archiveBytes []byte, checksumBytes []byte) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest", "/releases/tags/v" + releaseVersion:
			writeUpdateReleaseJSON(t, w, r, releaseVersion, archiveName)
		case "/assets/archive":
			if archiveBytes == nil {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(archiveBytes)
		case "/assets/checksums":
			if checksumBytes == nil {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(checksumBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}

func writeUpdateReleaseJSON(t *testing.T, w http.ResponseWriter, r *http.Request, releaseVersion string, archiveName string) {
	t.Helper()

	baseURL := "http://" + r.Host
	w.Header().Set("Content-Type", "application/json")
	_, err := fmt.Fprintf(w, `{
  "tag_name": "v%s",
  "assets": [
    {"name": %q, "browser_download_url": %q},
    {"name": "checksums.txt", "browser_download_url": %q}
  ]
}`, releaseVersion, archiveName, baseURL+"/assets/archive", baseURL+"/assets/checksums")
	if err != nil {
		t.Fatalf("write release JSON: %v", err)
	}
}

func makeUpdateZip(t *testing.T, name string, data []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	writer, err := zipWriter.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("write zip data: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buffer.Bytes()
}

func makeUpdateTarGz(t *testing.T, name string, data []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatalf("write tar data: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buffer.Bytes()
}

func sha256String(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

func prepareInstalledUpdateTarget(t *testing.T, content string) installPaths {
	t.Helper()

	tmpDir := t.TempDir()
	paths, err := resolveInstallPaths(filepath.Join(tmpDir, "bin"), filepath.Join(tmpDir, "lib"))
	if err != nil {
		t.Fatalf("resolve install paths: %v", err)
	}
	if err := os.MkdirAll(paths.BinDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	if err := os.MkdirAll(paths.LibDir, 0o755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	if err := os.WriteFile(paths.Target, []byte(content), 0o755); err != nil {
		t.Fatalf("write installed target: %v", err)
	}
	return paths
}
