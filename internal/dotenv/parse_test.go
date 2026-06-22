package dotenv

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDotenv(t *testing.T) {
	input := `
KEY=value
DOUBLE="quoted value"
SINGLE='single quoted value'
SPACES=value with spaces
# comment
EMPTY=
`

	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := map[string]string{
		"KEY":    "value",
		"DOUBLE": "quoted value",
		"SINGLE": "single quoted value",
		"SPACES": "value with spaces",
		"EMPTY":  "",
	}

	if len(got) != len(want) {
		t.Fatalf("len(Parse()) = %d, want %d", len(got), len(want))
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("Parse()[%s] = %q, want %q", key, got[key], wantValue)
		}
	}
}

func TestParseDotenvEscapesAndWhitespace(t *testing.T) {
	input := `
  KEY = value
DOUBLE_ESCAPES="line\nnext\tTabbed\"quoted\"\\slash"
SINGLE_LITERAL='line\nnot escaped'
UNKNOWN_ESCAPE="hello\q"
`

	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := map[string]string{
		"KEY":            "value",
		"DOUBLE_ESCAPES": "line\nnext\tTabbed\"quoted\"\\slash",
		"SINGLE_LITERAL": `line\nnot escaped`,
		"UNKNOWN_ESCAPE": "helloq",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseLastDuplicateKeyWins(t *testing.T) {
	got, err := Parse(strings.NewReader("TOKEN=old\nTOKEN=new\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got["TOKEN"] != "new" {
		t.Fatalf("TOKEN = %q, want new", got["TOKEN"])
	}
}

func TestParseRejectsInvalidKeys(t *testing.T) {
	tests := []string{
		"1KEY=value\n",
		"BAD-KEY=value\n",
		"=value\n",
	}

	for _, input := range tests {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			_, err := Parse(strings.NewReader(input))
			if err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
		})
	}
}

func TestParseRejectsUnterminatedQuotesWithoutLeakingValues(t *testing.T) {
	tests := []string{
		`API_KEY="super-secret`,
		`API_KEY='super-secret`,
		`API_KEY="super-secret\`,
	}

	for _, input := range tests {
		t.Run(input[:8], func(t *testing.T) {
			_, err := Parse(strings.NewReader(input))
			if err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
			if strings.Contains(err.Error(), "super-secret") {
				t.Fatalf("error leaked secret value: %v", err)
			}
		})
	}
}

func TestParseRejectsInvalidLinesWithoutLeakingValues(t *testing.T) {
	_, err := Parse(strings.NewReader("API_KEY\n"))
	if err == nil {
		t.Fatal("Parse() error = nil, want error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked secret value: %v", err)
	}
}
