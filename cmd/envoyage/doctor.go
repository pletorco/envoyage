package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/swoogi/envoyage/internal/dotenv"
	"github.com/swoogi/envoyage/internal/envops"
)

const (
	doctorCommandTimeout = 5 * time.Second
	doctorActionFormat   = "  action: %s\n"
	doctorCommandFormat  = "  command: %s\n"
	doctorReasonFormat   = "  reason: %s\n"
)

var (
	dockerRuntimePackages = []string{
		"docker-ce",
		"docker-ce-cli",
		"docker-compose-plugin",
		"docker-buildx-plugin",
		"containerd.io",
		"docker.io",
		"containerd",
		"runc",
	}
	podmanRuntimePackages = []string{
		"podman",
		"buildah",
		"skopeo",
		"conmon",
		"crun",
		"runc",
		"netavark",
		"aardvark-dns",
	}
)

type doctorOptions struct {
	RuntimeOnly bool
}

type doctorEnvironment struct {
	LookPath func(file string) (string, error)
	Run      func(ctx context.Context, name string, args ...string) (string, error)
	Update   func(ctx context.Context) envoyageUpdateDoctorReport
}

type envoyageUpdateDoctorReport struct {
	Latest  string
	Action  string
	Command string
	Reason  string
}

type runtimeDoctorReport struct {
	Name             string
	Installed        bool
	Binary           string
	Version          string
	ComposeVersion   string
	PackageManager   string
	Packages         []runtimePackageStatus
	UpdateCommand    string
	Action           string
	Reason           string
	Reference        string
	PackageCheckNote string
}

type runtimePackageStatus struct {
	Name      string
	Installed string
	Candidate string
	Update    bool
}

func runDoctor(args []string, stdout io.Writer) error {
	opts := doctorOptions{}
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.BoolVar(&opts.RuntimeOnly, "runtime", false, "check Docker and Podman runtime packages")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("doctor does not accept arguments")
	}

	env := doctorEnvironment{
		LookPath: exec.LookPath,
		Run:      runDoctorCommand,
		Update:   checkEnvoyageUpdateForDoctor,
	}
	return writeDoctorReport(context.Background(), opts, env, stdout)
}

func runDoctorCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, doctorCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		return text, fmt.Errorf("%s timed out", name)
	}
	if err != nil {
		return text, err
	}
	return text, nil
}

func writeDoctorReport(ctx context.Context, opts doctorOptions, env doctorEnvironment, stdout io.Writer) error {
	writeEnvoyageDoctorReport(ctx, opts, env, stdout)
	if err := writeRuntimeDoctorReports(ctx, opts, env, stdout); err != nil {
		return err
	}
	if !opts.RuntimeOnly {
		fmt.Fprintln(stdout, "")
		writeProjectDoctorReport(stdout, buildProjectDoctorReport())
	}
	return nil
}

func writeEnvoyageDoctorReport(ctx context.Context, opts doctorOptions, env doctorEnvironment, stdout io.Writer) {
	fmt.Fprintf(stdout, "Envoyage:\n  version: %s\n", version)
	if opts.RuntimeOnly {
		fmt.Fprintln(stdout, "")
		return
	}
	report := checkEnvoyageUpdate(ctx, env)
	if report.Latest != "" {
		fmt.Fprintf(stdout, "  latest: %s\n", report.Latest)
	}
	fmt.Fprintf(stdout, "  update: %s\n", report.Action)
	if report.Command != "" {
		fmt.Fprintf(stdout, doctorCommandFormat, report.Command)
	}
	if report.Reason != "" {
		fmt.Fprintf(stdout, doctorReasonFormat, report.Reason)
	}
	fmt.Fprintln(stdout, "")
}

func checkEnvoyageUpdate(ctx context.Context, env doctorEnvironment) envoyageUpdateDoctorReport {
	if env.Update != nil {
		return env.Update(ctx)
	}
	return checkEnvoyageUpdateForDoctor(ctx)
}

func checkEnvoyageUpdateForDoctor(ctx context.Context) envoyageUpdateDoctorReport {
	ctx, cancel := context.WithTimeout(ctx, doctorCommandTimeout)
	defer cancel()

	release, err := fetchRelease(ctx, updateHTTPClient, updateAPIBaseURL, "")
	if err != nil {
		return envoyageUpdateDoctorReport{
			Action: "unable to check",
			Reason: "GitHub release metadata could not be fetched",
		}
	}
	latest, err := updateTargetVersion(release, "")
	if err != nil {
		return envoyageUpdateDoctorReport{
			Action: "unable to check",
			Reason: err.Error(),
		}
	}
	if compareVersions(version, latest) >= 0 {
		return envoyageUpdateDoctorReport{
			Latest: latest,
			Action: "no update detected",
		}
	}
	return envoyageUpdateDoctorReport{
		Latest:  latest,
		Action:  "update available",
		Command: "envoyage update",
	}
}

func writeRuntimeDoctorReports(ctx context.Context, opts doctorOptions, env doctorEnvironment, stdout io.Writer) error {
	fmt.Fprintln(stdout, "Runtime Update Check:")
	fmt.Fprintf(stdout, "  scope: %s\n", doctorScope(opts))
	fmt.Fprintln(stdout, "  method: package manager update metadata")
	fmt.Fprintln(stdout, "  cve matching: not performed")
	fmt.Fprintln(stdout, "")

	reports := []runtimeDoctorReport{
		buildDockerDoctorReport(ctx, env),
		buildPodmanDoctorReport(ctx, env),
	}
	for i, report := range reports {
		if i > 0 {
			fmt.Fprintln(stdout, "")
		}
		writeRuntimeDoctorReport(stdout, report)
	}
	return nil
}

func doctorScope(opts doctorOptions) string {
	if opts.RuntimeOnly {
		return "runtime"
	}
	return "all"
}

func buildDockerDoctorReport(ctx context.Context, env doctorEnvironment) runtimeDoctorReport {
	report := runtimeDoctorReport{
		Name:      "Docker",
		Reference: "https://docs.docker.com/security/security-announcements/",
	}
	path, err := env.LookPath("docker")
	if err != nil {
		report.Action = "not installed"
		return report
	}
	report.Installed = true
	report.Binary = path
	report.Version = firstSuccessfulOutput(ctx, env, []doctorCommand{
		{Name: "docker", Args: []string{"version", "--format", "{{.Client.Version}}"}},
		{Name: "docker", Args: []string{"--version"}},
	})
	report.ComposeVersion = firstSuccessfulOutput(ctx, env, []doctorCommand{
		{Name: "docker", Args: []string{"compose", "version", "--short"}},
		{Name: "docker", Args: []string{"compose", "version"}},
	})
	applyPackageUpdateCheck(ctx, env, &report, dockerRuntimePackages)
	return report
}

func buildPodmanDoctorReport(ctx context.Context, env doctorEnvironment) runtimeDoctorReport {
	report := runtimeDoctorReport{
		Name:      "Podman",
		Reference: "https://podman.io/",
	}
	path, err := env.LookPath("podman")
	if err != nil {
		report.Action = "not installed"
		return report
	}
	report.Installed = true
	report.Binary = path
	report.Version = firstSuccessfulOutput(ctx, env, []doctorCommand{
		{Name: "podman", Args: []string{"version", "--format", "{{.Client.Version}}"}},
		{Name: "podman", Args: []string{"--version"}},
	})
	applyPackageUpdateCheck(ctx, env, &report, podmanRuntimePackages)
	return report
}

type doctorCommand struct {
	Name string
	Args []string
}

func firstSuccessfulOutput(ctx context.Context, env doctorEnvironment, commands []doctorCommand) string {
	for _, command := range commands {
		output, err := env.Run(ctx, command.Name, command.Args...)
		if err == nil && output != "" {
			return output
		}
	}
	return "unknown"
}

func applyPackageUpdateCheck(ctx context.Context, env doctorEnvironment, report *runtimeDoctorReport, packageNames []string) {
	checker := detectPackageChecker(env)
	if checker == nil {
		report.Action = "unable to verify package updates"
		report.Reason = "supported package manager not detected"
		return
	}

	report.PackageManager = checker.Name()
	report.Packages = checker.Check(ctx, env, packageNames)
	report.UpdateCommand = checker.UpdateCommand(updatedPackageNames(report.Packages))
	if len(report.Packages) == 0 {
		report.Action = "unable to verify package updates"
		report.Reason = "no related runtime packages were found in the package manager"
		return
	}
	if runtimePackagesHaveUpdates(report.Packages) {
		report.Action = "update recommended"
		return
	}
	report.Action = "no runtime package update detected"
}

type packageChecker interface {
	Name() string
	Check(ctx context.Context, env doctorEnvironment, packageNames []string) []runtimePackageStatus
	UpdateCommand(packageNames []string) string
}

func detectPackageChecker(env doctorEnvironment) packageChecker {
	if commandAvailable(env, "dpkg-query") && commandAvailable(env, "apt-cache") {
		return debianPackageChecker{}
	}
	if commandAvailable(env, "rpm") && commandAvailable(env, "dnf") {
		return rpmPackageChecker{manager: "dnf"}
	}
	if commandAvailable(env, "rpm") && commandAvailable(env, "yum") {
		return rpmPackageChecker{manager: "yum"}
	}
	return nil
}

func commandAvailable(env doctorEnvironment, name string) bool {
	_, err := env.LookPath(name)
	return err == nil
}

type debianPackageChecker struct{}

func (debianPackageChecker) Name() string {
	return "apt/dpkg"
}

func (debianPackageChecker) Check(ctx context.Context, env doctorEnvironment, packageNames []string) []runtimePackageStatus {
	var statuses []runtimePackageStatus
	for _, name := range packageNames {
		status, ok := debianPackageStatus(ctx, env, name)
		if ok {
			statuses = append(statuses, status)
		}
	}
	return statuses
}

func (debianPackageChecker) UpdateCommand(packageNames []string) string {
	if len(packageNames) == 0 {
		return "sudo apt update && sudo apt upgrade"
	}
	return "sudo apt update && sudo apt install " + strings.Join(packageNames, " ")
}

func debianPackageStatus(ctx context.Context, env doctorEnvironment, name string) (runtimePackageStatus, bool) {
	installed, err := env.Run(ctx, "dpkg-query", "-W", "-f=${Version}", name)
	if err != nil || installed == "" {
		return runtimePackageStatus{}, false
	}
	candidate := debianPackageCandidate(ctx, env, name)
	status := runtimePackageStatus{Name: name, Installed: installed, Candidate: candidate}
	status.Update = candidate != "" && candidate != "(none)" && candidate != installed
	return status, true
}

func debianPackageCandidate(ctx context.Context, env doctorEnvironment, name string) string {
	output, err := env.Run(ctx, "apt-cache", "policy", name)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Candidate:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
		}
	}
	return ""
}

type rpmPackageChecker struct {
	manager string
}

func (checker rpmPackageChecker) Name() string {
	return checker.manager + "/rpm"
}

func (checker rpmPackageChecker) Check(ctx context.Context, env doctorEnvironment, packageNames []string) []runtimePackageStatus {
	var statuses []runtimePackageStatus
	for _, name := range packageNames {
		status, ok := rpmPackageStatus(ctx, env, checker.manager, name)
		if ok {
			statuses = append(statuses, status)
		}
	}
	return statuses
}

func (checker rpmPackageChecker) UpdateCommand(packageNames []string) string {
	if len(packageNames) == 0 {
		return "sudo " + checker.manager + " upgrade"
	}
	return "sudo " + checker.manager + " upgrade " + strings.Join(packageNames, " ")
}

func rpmPackageStatus(ctx context.Context, env doctorEnvironment, manager string, name string) (runtimePackageStatus, bool) {
	installed, err := env.Run(ctx, "rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", name)
	if err != nil || installed == "" {
		return runtimePackageStatus{}, false
	}
	candidate := rpmPackageCandidate(ctx, env, manager, name)
	status := runtimePackageStatus{Name: name, Installed: installed, Candidate: candidate}
	status.Update = candidate != "" && candidate != installed
	return status, true
}

func rpmPackageCandidate(ctx context.Context, env doctorEnvironment, manager string, name string) string {
	output, _ := env.Run(ctx, manager, "--quiet", "check-update", name)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasPrefix(fields[0], name+".") {
			return fields[1]
		}
	}
	return ""
}

func updatedPackageNames(statuses []runtimePackageStatus) []string {
	var names []string
	for _, status := range statuses {
		if status.Update {
			names = append(names, status.Name)
		}
	}
	return names
}

func runtimePackagesHaveUpdates(statuses []runtimePackageStatus) bool {
	for _, status := range statuses {
		if status.Update {
			return true
		}
	}
	return false
}

func writeRuntimeDoctorReport(stdout io.Writer, report runtimeDoctorReport) {
	fmt.Fprintf(stdout, "%s:\n", report.Name)
	if !report.Installed {
		fmt.Fprintln(stdout, "  installed: no")
		fmt.Fprintf(stdout, doctorActionFormat, report.Action)
		return
	}

	fmt.Fprintln(stdout, "  installed: yes")
	fmt.Fprintf(stdout, "  binary: %s\n", report.Binary)
	fmt.Fprintf(stdout, "  version: %s\n", report.Version)
	if report.ComposeVersion != "" {
		fmt.Fprintf(stdout, "  compose: %s\n", report.ComposeVersion)
	}
	if report.PackageManager != "" {
		fmt.Fprintf(stdout, "  package manager: %s\n", report.PackageManager)
	}
	writeRuntimePackages(stdout, report.Packages)
	fmt.Fprintf(stdout, doctorActionFormat, report.Action)
	if report.Reason != "" {
		fmt.Fprintf(stdout, doctorReasonFormat, report.Reason)
	}
	if report.UpdateCommand != "" && runtimePackagesHaveUpdates(report.Packages) {
		fmt.Fprintf(stdout, doctorCommandFormat, report.UpdateCommand)
	}
	if report.Reference != "" {
		fmt.Fprintf(stdout, "  reference: %s\n", report.Reference)
	}
}

func writeRuntimePackages(stdout io.Writer, packages []runtimePackageStatus) {
	if len(packages) == 0 {
		return
	}
	fmt.Fprintln(stdout, "  packages:")
	for _, pkg := range packages {
		if pkg.Update {
			fmt.Fprintf(stdout, "    %s: %s -> %s (update available)\n", pkg.Name, pkg.Installed, pkg.Candidate)
		} else {
			fmt.Fprintf(stdout, "    %s: %s (current candidate)\n", pkg.Name, pkg.Installed)
		}
	}
}

type projectDoctorReport struct {
	ComposePath       string
	EnvFound          bool
	SecretsFound      bool
	AgeFound          bool
	SecretsNewer      bool
	EnvSecretKeys     []string
	ExtractEnvKeys    []string
	ExtractSecretKeys []string
	Updates           []envops.Update
	Action            string
	Command           string
	Reason            string
}

func buildProjectDoctorReport() projectDoctorReport {
	report := projectDoctorReport{
		EnvFound:     fileExists(envops.DefaultEnvPath),
		SecretsFound: fileExists(envops.DefaultSecretsPath),
		AgeFound:     fileExists(envops.DefaultAgeEnvPath),
	}
	report.SecretsNewer = secretsNewerThanAge(envops.DefaultSecretsPath, envops.DefaultAgeEnvPath)
	extract, err := envops.Extract(envops.ExtractOptions{Secrets: true})
	if err != nil {
		return projectReportFromExtractError(report, err)
	}

	report.ComposePath = extract.ComposePath
	report.EnvSecretKeys = secretKeysInEnvFile(envops.DefaultEnvPath)
	report.ExtractEnvKeys = extract.EnvKeys
	report.ExtractSecretKeys = extract.SecretKeys
	report.Updates = extract.Updates
	return finalizeProjectReport(report)
}

func projectReportFromExtractError(report projectDoctorReport, err error) projectDoctorReport {
	message := err.Error()
	switch {
	case strings.Contains(message, "compose file not found"):
		report.Action = "no Compose file detected"
		return report
	case strings.Contains(message, "multiple compose files"):
		report.Action = "unable to check Compose env split"
		report.Reason = "multiple Compose files found; pass --compose to env extract when migrating"
	default:
		report.Action = "unable to check Compose env split"
		report.Reason = sanitizeDoctorReason(message)
	}
	return finalizeProjectReport(report)
}

func finalizeProjectReport(report projectDoctorReport) projectDoctorReport {
	if len(report.EnvSecretKeys) > 0 {
		report.Action = "move secrets from .env recommended"
		report.Command = "move secret-looking keys to .secrets.env && envoyage encrypt"
		return report
	}
	if len(report.ExtractSecretKeys) > 0 {
		report.Action = "secret extraction recommended"
		report.Command = "envoyage env extract --write --secrets && envoyage encrypt"
		return report
	}
	if len(report.ExtractEnvKeys) > 0 {
		report.Action = "env extraction recommended"
		report.Command = "envoyage env extract --write"
		return report
	}
	if report.Action == "" {
		report.Action = "no env extraction needed"
	}
	if report.SecretsFound && !report.AgeFound {
		report.Action = "encrypt recommended"
		report.Command = "envoyage encrypt"
		report.Reason = ".secrets.env exists but .env.age was not found"
	}
	if report.SecretsNewer {
		report.Action = "encrypt recommended"
		report.Command = "envoyage encrypt --force"
		report.Reason = ".secrets.env is newer than .env.age"
	}
	return report
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func secretsNewerThanAge(secretsPath string, agePath string) bool {
	secretsInfo, secretsErr := os.Stat(secretsPath)
	ageInfo, ageErr := os.Stat(agePath)
	if secretsErr != nil || ageErr != nil {
		return false
	}
	return secretsInfo.ModTime().After(ageInfo.ModTime())
}

func secretKeysInEnvFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	values, err := dotenv.Parse(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	var keys []string
	for key := range values {
		if doctorSecretLikeKey(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func doctorSecretLikeKey(key string) bool {
	upper := strings.ToUpper(key)
	patterns := []string{
		"PASSWORD",
		"PASS",
		"SECRET",
		"TOKEN",
		"API_KEY",
		"PRIVATE_KEY",
		"ACCESS_KEY",
		"CREDENTIAL",
	}
	for _, pattern := range patterns {
		if strings.Contains(upper, pattern) {
			return true
		}
	}
	return false
}

func sanitizeDoctorReason(reason string) string {
	if idx := strings.Index(reason, "\n"); idx >= 0 {
		return reason[:idx]
	}
	return reason
}

func writeProjectDoctorReport(stdout io.Writer, report projectDoctorReport) {
	fmt.Fprintln(stdout, "Project Env Check:")
	if report.ComposePath != "" {
		fmt.Fprintf(stdout, "  compose: %s\n", report.ComposePath)
	} else {
		fmt.Fprintln(stdout, "  compose: not found")
	}
	fmt.Fprintf(stdout, "  .env: %s\n", foundText(report.EnvFound))
	fmt.Fprintf(stdout, "  .secrets.env: %s\n", foundText(report.SecretsFound))
	fmt.Fprintf(stdout, "  .env.age: %s\n", foundText(report.AgeFound))
	writeDoctorKeyLine(stdout, "  fixed env keys", report.ExtractEnvKeys)
	writeDoctorKeyLine(stdout, "  fixed secret keys", report.ExtractSecretKeys)
	writeDoctorKeyLine(stdout, "  .env secret-looking keys", report.EnvSecretKeys)
	fmt.Fprintf(stdout, doctorActionFormat, report.Action)
	if report.Command != "" {
		fmt.Fprintf(stdout, doctorCommandFormat, report.Command)
	}
	if report.Reason != "" {
		fmt.Fprintf(stdout, doctorReasonFormat, report.Reason)
	}
}

func foundText(found bool) string {
	if found {
		return "found"
	}
	return "missing"
}

func writeDoctorKeyLine(stdout io.Writer, label string, keys []string) {
	if len(keys) == 0 {
		return
	}
	fmt.Fprintf(stdout, "%s: %s\n", label, strings.Join(keys, ", "))
}
