package dotenv

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// Parse reads a small, Docker Compose-friendly subset of dotenv syntax.
func Parse(r io.Reader) (map[string]string, error) {
	env := make(map[string]string)
	scanner := bufio.NewScanner(r)
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: missing '='", lineNo)
		}

		key = strings.TrimSpace(key)
		if !validKey(key) {
			return nil, fmt.Errorf("line %d: invalid key %q", lineNo, key)
		}

		parsed, err := parseValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("line %d key %s: %w", lineNo, key, err)
		}
		env[key] = parsed
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return env, nil
}

func parseValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	switch value[0] {
	case '\'':
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", fmt.Errorf("unterminated single-quoted value")
		}
		return value[1 : len(value)-1], nil
	case '"':
		if len(value) < 2 || value[len(value)-1] != '"' {
			return "", fmt.Errorf("unterminated double-quoted value")
		}
		return unescapeDoubleQuoted(value[1 : len(value)-1])
	default:
		return value, nil
	}
}

func unescapeDoubleQuoted(value string) (string, error) {
	var b strings.Builder
	b.Grow(len(value))

	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			b.WriteByte(value[i])
			continue
		}
		if i == len(value)-1 {
			return "", fmt.Errorf("dangling escape")
		}
		i++
		switch value[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\', '"':
			b.WriteByte(value[i])
		default:
			b.WriteByte(value[i])
		}
	}

	return b.String(), nil
}

func validKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if i == 0 && !(r == '_' || unicode.IsLetter(r)) {
			return false
		}
		if !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
