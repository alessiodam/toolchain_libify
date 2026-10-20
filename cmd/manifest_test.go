package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func manifestFlags() (*pflag.FlagSet, *string, *[]string) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	name := flags.String("name", "", "")
	depend := flags.StringSlice("depend", nil, "")
	return flags, name, depend
}

func TestManifestFillsUnsetFlags(t *testing.T) {
	path := write(t, manifestFile, strings.Join([]string{
		`name = "EXAMPLE"`,
		`depend = ["usbdrvce", "fileioc"]`,
		`optimize = "-O2"`,
		`[app]`,
		`name = "ExampleApp"`,
		`init = "example_init_runtime"`,
		`imports = ["usb_Init"]`,
		`[app.make]`,
		`LTO = "NO"`,
		`[[step]]`,
		`run = ["python3", "headers.py"]`,
	}, "\n"))

	flags, name, depend := manifestFlags()
	if err := flags.Parse([]string{"--name", "OVERRIDE"}); err != nil {
		t.Fatal(err)
	}
	if err := applyManifest(flags, path); err != nil {
		t.Fatalf("applyManifest: %v", err)
	}

	if *name != "OVERRIDE" {
		t.Fatalf("command line flag lost to manifest: %q", *name)
	}
	if strings.Join(*depend, ",") != "usbdrvce,fileioc" {
		t.Fatalf("depend = %v", *depend)
	}
	if app == nil || app.Name != "ExampleApp" || app.Make["LTO"] != "NO" || app.Imports[0] != "usb_Init" {
		t.Fatalf("app = %+v", app)
	}
	if len(steps) != 1 || steps[0].Run[1] != "headers.py" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestManifestRejectsUnknownKeys(t *testing.T) {
	path := write(t, manifestFile, `nmae = "DEMO"`)

	flags, _, _ := manifestFlags()
	if err := applyManifest(flags, path); err == nil || !strings.Contains(err.Error(), "nmae") {
		t.Fatalf("got %v", err)
	}
}

func TestManifestIsOptional(t *testing.T) {
	flags, _, _ := manifestFlags()
	if err := applyManifest(flags, filepath.Join(t.TempDir(), manifestFile)); err != nil {
		t.Fatal(err)
	}
	if app != nil || steps != nil {
		t.Fatalf("app = %+v, steps = %+v", app, steps)
	}
}

func TestSourcesWalkAndExclude(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"a.c", "core/b.c", "core/b.h", "contrib/x/main.c", "tls/main.c"} {
		path := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	p := &pipeline{srcDir: root, exclude: []string{"contrib", "*/main.c"}}
	files, err := p.sources(".c")
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, file := range files {
		rel, _ := filepath.Rel(root, file)
		got = append(got, filepath.ToSlash(rel))
	}
	if strings.Join(got, " ") != "a.c core/b.c" {
		t.Fatalf("got %v", got)
	}
}
