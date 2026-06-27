package envops

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/swoogi/envoyage/internal/compose"
	"github.com/swoogi/envoyage/internal/dotenv"
	"gopkg.in/yaml.v3"
)

const (
	DefaultEnvPath     = ".env"
	DefaultSecretsPath = ".secrets.env"
	DefaultAgeEnvPath  = ".env.age"
)

var defaultComposeFiles = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var variablePattern = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?+]).*)?\}$`)

type ExtractOptions struct {
	ComposePath string
	EnvPath     string
	SecretsPath string
	Write       bool
	Secrets     bool
}

type InlineOptions struct {
	ComposePath  string
	OutputPath   string
	EnvFiles     []string
	IdentityFile string
	Force        bool
}

type Result struct {
	ComposePath string
	EnvKeys     []string
	SecretKeys  []string
	Updates     []Update
}

type Update struct {
	Service string
	Key     string
}

type plannedValue struct {
	Value  string
	Secret bool
}

func Extract(opts ExtractOptions) (Result, error) {
	if opts.EnvPath == "" {
		opts.EnvPath = DefaultEnvPath
	}
	if opts.SecretsPath == "" {
		opts.SecretsPath = DefaultSecretsPath
	}

	composePath, err := resolveComposePath(opts.ComposePath)
	if err != nil {
		return Result{}, err
	}
	doc, err := readComposeDocument(composePath)
	if err != nil {
		return Result{}, err
	}

	planned := make(map[string]plannedValue)
	var updates []Update
	if err := walkServiceEnvironments(doc, func(service string, envNode *yaml.Node) error {
		serviceUpdates, err := extractEnvironment(service, envNode, planned, opts.Secrets)
		if err != nil {
			return err
		}
		updates = append(updates, serviceUpdates...)
		return nil
	}); err != nil {
		return Result{}, err
	}

	result := resultFromPlanned(composePath, planned, updates)
	if opts.Write {
		if err := appendEnvFiles(opts.EnvPath, opts.SecretsPath, planned); err != nil {
			return Result{}, err
		}
		if err := writeComposeDocument(composePath, doc, 0o644, true); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func Inline(opts InlineOptions) (Result, error) {
	if opts.OutputPath == "" {
		return Result{}, fmt.Errorf("--out is required\n\nhint:\n  write rendered compose output to a separate file, for example: envoyage env inline --out compose.inline.yaml")
	}

	composePath, err := resolveComposePath(opts.ComposePath)
	if err != nil {
		return Result{}, err
	}
	if sameCleanPath(composePath, opts.OutputPath) {
		return Result{}, fmt.Errorf("--out must be different from the source compose file\n\nhint:\n  inline output may contain plaintext secrets; use a separate file such as compose.inline.yaml")
	}

	envFiles := opts.EnvFiles
	if len(envFiles) == 0 {
		envFiles, err = existingEnvFiles([]string{DefaultEnvPath, DefaultSecretsPath, DefaultAgeEnvPath})
		if err != nil {
			return Result{}, err
		}
	}
	env, err := compose.LoadEnvFiles(envFiles, opts.IdentityFile)
	if err != nil {
		return Result{}, err
	}

	doc, err := readComposeDocument(composePath)
	if err != nil {
		return Result{}, err
	}

	var updates []Update
	if err := walkServiceEnvironments(doc, func(service string, envNode *yaml.Node) error {
		updates = append(updates, inlineEnvironment(service, envNode, env)...)
		return nil
	}); err != nil {
		return Result{}, err
	}

	if err := writeComposeDocument(opts.OutputPath, doc, 0o600, opts.Force); err != nil {
		return Result{}, err
	}
	return Result{ComposePath: composePath, Updates: updates}, nil
}

func resolveComposePath(path string) (string, error) {
	if path != "" {
		return path, nil
	}

	var found []string
	for _, candidate := range defaultComposeFiles {
		_, err := os.Stat(candidate)
		switch {
		case err == nil:
			found = append(found, candidate)
		case os.IsNotExist(err):
			continue
		default:
			return "", fmt.Errorf("stat compose file %s: %w", candidate, err)
		}
	}
	if len(found) == 0 {
		return "", fmt.Errorf("compose file not found; pass --compose PATH\n\nhint:\n  supported default names are compose.yaml, compose.yml, docker-compose.yaml, and docker-compose.yml")
	}
	if len(found) > 1 {
		return "", fmt.Errorf("multiple compose files found; pass --compose PATH\n\nhint:\n  choose the source file explicitly, for example: envoyage env extract --compose compose.yaml")
	}
	return found[0], nil
}

func readComposeDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read compose file %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse compose file %s: %w", path, err)
	}
	return &doc, nil
}

func writeComposeDocument(path string, doc *yaml.Node, mode os.FileMode, force bool) error {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		encoder.Close()
		return fmt.Errorf("render compose file: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("render compose file: %w", err)
	}

	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !force {
		flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("output file %s already exists; pass --force to overwrite it\n\nhint:\n  inline output may contain plaintext secrets; verify the target path before using --force", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect output file %s: %w", path, err)
		}
	}
	file, err := os.OpenFile(path, flag, mode)
	if err != nil {
		return fmt.Errorf("create output file %s: %w", path, err)
	}
	defer file.Close()
	if _, err := file.Write(out.Bytes()); err != nil {
		return fmt.Errorf("write output file %s: %w", path, err)
	}
	return nil
}

func walkServiceEnvironments(doc *yaml.Node, fn func(service string, envNode *yaml.Node) error) error {
	root := documentContent(doc)
	services := mappingValue(root, "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(services.Content); i += 2 {
		serviceName := services.Content[i].Value
		service := services.Content[i+1]
		if service.Kind != yaml.MappingNode {
			continue
		}
		envNode := mappingValue(service, "environment")
		if envNode == nil {
			continue
		}
		if err := fn(serviceName, envNode); err != nil {
			return err
		}
	}
	return nil
}

func extractEnvironment(service string, envNode *yaml.Node, planned map[string]plannedValue, secrets bool) ([]Update, error) {
	switch envNode.Kind {
	case yaml.MappingNode:
		return extractMappingEnvironment(service, envNode, planned, secrets)
	case yaml.SequenceNode:
		return extractSequenceEnvironment(service, envNode, planned, secrets)
	default:
		return nil, nil
	}
}

func extractMappingEnvironment(service string, envNode *yaml.Node, planned map[string]plannedValue, secrets bool) ([]Update, error) {
	var updates []Update
	for i := 0; i+1 < len(envNode.Content); i += 2 {
		key := envNode.Content[i].Value
		valueNode := envNode.Content[i+1]
		if !extractableKeyValue(key, valueNode) {
			continue
		}
		if err := addPlannedValue(planned, key, valueNode.Value, secrets && secretLikeKey(key)); err != nil {
			return nil, err
		}
		setVariableReference(valueNode, key)
		updates = append(updates, Update{Service: service, Key: key})
	}
	return updates, nil
}

func extractSequenceEnvironment(service string, envNode *yaml.Node, planned map[string]plannedValue, secrets bool) ([]Update, error) {
	var updates []Update
	for _, item := range envNode.Content {
		key, value, ok := extractableListValue(item)
		if !ok {
			continue
		}
		if err := addPlannedValue(planned, key, value, secrets && secretLikeKey(key)); err != nil {
			return nil, err
		}
		item.Value = key + "=${" + key + "}"
		item.Tag = "!!str"
		item.Style = 0
		updates = append(updates, Update{Service: service, Key: key})
	}
	return updates, nil
}

func inlineEnvironment(service string, envNode *yaml.Node, env map[string]string) []Update {
	switch envNode.Kind {
	case yaml.MappingNode:
		return inlineMappingEnvironment(service, envNode, env)
	case yaml.SequenceNode:
		return inlineSequenceEnvironment(service, envNode, env)
	default:
		return nil
	}
}

func inlineMappingEnvironment(service string, envNode *yaml.Node, env map[string]string) []Update {
	var updates []Update
	for i := 0; i+1 < len(envNode.Content); i += 2 {
		key := envNode.Content[i].Value
		valueNode := envNode.Content[i+1]
		value, ok := inlineValue(valueNode, env)
		if !ok {
			continue
		}
		valueNode.Value = value
		valueNode.Tag = "!!str"
		valueNode.Style = 0
		updates = append(updates, Update{Service: service, Key: key})
	}
	return updates
}

func inlineSequenceEnvironment(service string, envNode *yaml.Node, env map[string]string) []Update {
	var updates []Update
	for _, item := range envNode.Content {
		key, value, ok := strings.Cut(item.Value, "=")
		if !ok {
			continue
		}
		resolved, ok := inlineRawValue(value, env)
		if !ok {
			continue
		}
		item.Value = key + "=" + resolved
		item.Tag = "!!str"
		item.Style = 0
		updates = append(updates, Update{Service: service, Key: key})
	}
	return updates
}

func extractableKeyValue(key string, valueNode *yaml.Node) bool {
	return validEnvKey(key) && fixedScalar(valueNode) && !wholeVariableReference(valueNode.Value)
}

func extractableListValue(item *yaml.Node) (string, string, bool) {
	if !fixedScalar(item) {
		return "", "", false
	}
	key, value, ok := strings.Cut(item.Value, "=")
	if !ok || !validEnvKey(key) || wholeVariableReference(value) {
		return "", "", false
	}
	return key, value, true
}

func setVariableReference(node *yaml.Node, key string) {
	node.Kind = yaml.ScalarNode
	node.Tag = "!!str"
	node.Value = "${" + key + "}"
	node.Style = 0
}

func inlineValue(node *yaml.Node, env map[string]string) (string, bool) {
	if !fixedScalar(node) {
		return "", false
	}
	return inlineRawValue(node.Value, env)
}

func inlineRawValue(value string, env map[string]string) (string, bool) {
	envKey, ok := variableName(value)
	if !ok {
		return "", false
	}
	resolved, exists := env[envKey]
	return resolved, exists
}

func addPlannedValue(planned map[string]plannedValue, key string, value string, secret bool) error {
	if existing, ok := planned[key]; ok {
		if existing.Value != value {
			return fmt.Errorf("conflicting values for key %s\n\nhint:\n  use distinct environment variable names or make the fixed Compose values match before extracting", key)
		}
		existing.Secret = existing.Secret || secret
		planned[key] = existing
		return nil
	}
	planned[key] = plannedValue{Value: value, Secret: secret}
	return nil
}

func appendEnvFiles(envPath string, secretsPath string, planned map[string]plannedValue) error {
	envValues := make(map[string]string)
	secretValues := make(map[string]string)
	for key, value := range planned {
		if value.Secret {
			secretValues[key] = value.Value
		} else {
			envValues[key] = value.Value
		}
	}
	if err := appendEnvFile(envPath, envValues, 0o644); err != nil {
		return err
	}
	if err := appendEnvFile(secretsPath, secretValues, 0o600); err != nil {
		return err
	}
	return nil
}

func appendEnvFile(path string, values map[string]string, mode os.FileMode) error {
	if len(values) == 0 {
		return nil
	}

	data, existing, err := readExistingEnvFile(path)
	if err != nil {
		return err
	}
	missing, err := missingEnvKeys(path, existing, values)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}

	rendered := renderAppendedEnv(data, missing, values)
	return writeEnvBytes(path, rendered, mode)
}

func readExistingEnvFile(path string) ([]byte, map[string]string, error) {
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		parsed, parseErr := dotenv.Parse(bytes.NewReader(data))
		if parseErr != nil {
			return nil, nil, fmt.Errorf("parse existing env file %s: %w", path, parseErr)
		}
		return data, parsed, nil
	case os.IsNotExist(err):
		return nil, map[string]string{}, nil
	default:
		return nil, nil, fmt.Errorf("read existing env file %s: %w", path, err)
	}
}

func missingEnvKeys(path string, existing map[string]string, values map[string]string) ([]string, error) {
	var missing []string
	for _, key := range sortedKeys(values) {
		existingValue, ok := existing[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		if existingValue != values[key] {
			return nil, fmt.Errorf("existing env file %s has a different value for key %s\n\nhint:\n  update the existing env file manually or remove that key before running env extract --write", path, key)
		}
	}
	return missing, nil
}

func renderAppendedEnv(data []byte, missing []string, values map[string]string) []byte {
	var b bytes.Buffer
	b.Write(data)
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		b.WriteByte('\n')
	}
	for _, key := range missing {
		fmt.Fprintf(&b, "%s=%s\n", key, formatDotenvValue(values[key]))
	}
	return b.Bytes()
}

func writeEnvBytes(path string, data []byte, mode os.FileMode) error {
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	file, err := os.OpenFile(path, flag, mode)
	if err != nil {
		return fmt.Errorf("write env file %s: %w", path, err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write env file %s: %w", path, err)
	}
	return nil
}

func resultFromPlanned(composePath string, planned map[string]plannedValue, updates []Update) Result {
	var envKeys []string
	var secretKeys []string
	for key, value := range planned {
		if value.Secret {
			secretKeys = append(secretKeys, key)
		} else {
			envKeys = append(envKeys, key)
		}
	}
	sort.Strings(envKeys)
	sort.Strings(secretKeys)
	return Result{
		ComposePath: composePath,
		EnvKeys:     envKeys,
		SecretKeys:  secretKeys,
		Updates:     updates,
	}
}

func documentContent(doc *yaml.Node) *yaml.Node {
	if doc == nil {
		return nil
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func fixedScalar(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Tag != "!!null"
}

func wholeVariableReference(value string) bool {
	_, ok := variableName(value)
	return ok
}

func variableName(value string) (string, bool) {
	matches := variablePattern.FindStringSubmatch(value)
	if matches == nil {
		return "", false
	}
	return matches[1], true
}

func validEnvKey(key string) bool {
	return envKeyPattern.MatchString(key)
}

func secretLikeKey(key string) bool {
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

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatDotenvValue(value string) string {
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t\r\n#'\"\\") {
		escaped := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\t", "\\t", "\"", "\\\"").Replace(value)
		return `"` + escaped + `"`
	}
	return value
}

func existingEnvFiles(paths []string) ([]string, error) {
	var found []string
	for _, path := range paths {
		_, err := os.Stat(path)
		switch {
		case err == nil:
			found = append(found, path)
		case os.IsNotExist(err):
			continue
		default:
			return nil, fmt.Errorf("stat env file %s: %w", path, err)
		}
	}
	return found, nil
}

func sameCleanPath(left string, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightAbs == "" || rightErr != nil || leftAbs == "" {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return leftAbs == rightAbs
}
