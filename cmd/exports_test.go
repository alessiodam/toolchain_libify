package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, name, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestReadExportsSkipsBlanksAndComments(t *testing.T) {
	path := write(t, "exports.txt", strings.Join([]string{
		"# a header",
		"",
		"demo_start",
		"# a section",
		"demo_stop",
		"",
	}, "\n"))

	names, err := readExports(path)
	if err != nil {
		t.Fatalf("readExports: %v", err)
	}

	if len(names) != 2 || names[0] != "demo_start" || names[1] != "demo_stop" {
		t.Fatalf("got %v", names)
	}
}

func TestReadExportsRejectsDuplicates(t *testing.T) {
	path := write(t, "exports.txt", "demo_start\ndemo_start\n")

	if _, err := readExports(path); err == nil {
		t.Fatal("a duplicate export would claim two ordinals, expected an error")
	}
}

func TestReadExportsRejectsAnEmptyList(t *testing.T) {
	path := write(t, "exports.txt", "# nothing here\n")

	if _, err := readExports(path); err == nil {
		t.Fatal("expected an error for an empty list")
	}
}

func TestDeclaredInFindsFunctions(t *testing.T) {
	header := write(t, "demo.h", strings.Join([]string{
		"#ifndef DEMO_H",
		"#define DEMO_H",
		"typedef struct { int x; } demo_thing_t;",
		"bool demo_start(void);",
		"void demo_stop(void);",
		"const char *demo_name(demo_thing_t *thing);",
		"int other_helper(void);",
		"#endif",
	}, "\n"))

	names, err := declaredIn([]string{header}, []string{"demo_"})
	if err != nil {
		t.Fatalf("declaredIn: %v", err)
	}

	want := map[string]bool{"demo_start": true, "demo_stop": true, "demo_name": true}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for _, name := range names {
		if !want[name] {
			t.Errorf("unexpected declaration %q", name)
		}
	}
}

func TestDeclaredInWithoutPrefixesTakesEverything(t *testing.T) {
	header := write(t, "demo.h", "void alpha(void);\nvoid beta(void);\n")

	names, err := declaredIn([]string{header}, nil)
	if err != nil {
		t.Fatalf("declaredIn: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("got %v, want two declarations", names)
	}
}

func TestHasAnyPrefix(t *testing.T) {
	if !hasAnyPrefix("demo_start", nil) {
		t.Error("an empty prefix list should accept everything")
	}
	if !hasAnyPrefix("demo_start", []string{"other_", "demo_"}) {
		t.Error("should match the second prefix")
	}
	if hasAnyPrefix("demo_start", []string{"other_"}) {
		t.Error("should not match")
	}
}
