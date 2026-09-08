package libasm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestOSRoutine(t *testing.T) {
	cases := map[string]string{
		"__frameset0": "ti._frameset0",
		"__lmulu":     "ti._lmulu",
		"_memcpy":     "ti._memcpy",
		"usb_Init":    "",
	}

	for name, want := range cases {
		if got := OSRoutine(name); got != want {
			t.Errorf("OSRoutine(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestLoadOSSymbols(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ti84pceg.inc", strings.Join([]string{
		"define ti? ti",
		"namespace ti?",
		"?_memcpy                   := 00000A4h",
		"?_frameset0                := 0000130h",
		"; a comment",
		"end namespace",
	}, "\n"))

	names, err := LoadOSSymbols(dir)
	if err != nil {
		t.Fatalf("LoadOSSymbols: %v", err)
	}

	if !names["ti._memcpy"] || !names["ti._frameset0"] {
		t.Errorf("expected entries missing: %v", names)
	}
	if names["ti._nope"] {
		t.Errorf("invented an entry")
	}
}

func TestLoadDependency(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "usbdrvce.lib", strings.Join([]string{
		"\tlibrary\tUSBDRVCE, 0",
		"",
		"\texport\tusb_Init",
		"\texport\tusb_Cleanup",
	}, "\n"))

	dep, err := LoadDependency(path)
	if err != nil {
		t.Fatalf("LoadDependency: %v", err)
	}

	if dep.Name != "USBDRVCE" {
		t.Errorf("Name = %q, want USBDRVCE", dep.Name)
	}
	if len(dep.Exports) != 2 {
		t.Fatalf("Exports = %v, want two entries", dep.Exports)
	}

	renames := dep.Renames()
	if renames["_usb_Init"] != "usb_Init" {
		t.Errorf("Renames = %v", renames)
	}
}

func TestLoadDependencyRejectsAFileWithoutALibraryRecord(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "broken.lib", "\texport\tsomething\n")

	if _, err := LoadDependency(path); err == nil {
		t.Fatal("expected an error for a stub with no library record")
	}
}

func TestEmitOrdersTheImageForFasmg(t *testing.T) {
	dir := t.TempDir()
	stub := writeFile(t, dir, "usbdrvce.lib", "\tlibrary\tUSBDRVCE, 0\n\texport\tusb_Init\n")

	dep, err := LoadDependency(stub)
	if err != nil {
		t.Fatalf("LoadDependency: %v", err)
	}

	image := string(Emit(Options{
		Name:         "DEMO",
		Version:      3,
		VendorDir:    "vendor",
		Dependencies: []*Dependency{dep},
		Exports:      []string{"demo_start", "demo_stop"},
		Aliases:      map[string]string{"_memcpy": "ti._memcpy"},
		Stubs:        []string{"__indcallhl"},
		Body:         []byte("_demo_start:\n\tret\n_demo_stop:\n\tret\n"),
	}))

	mustContain := []string{
		"library DEMO, 3",
		"include_library '" + filepath.ToSlash(stub) + "'",
		"_memcpy := ti._memcpy",
		"__indcallhl:",
		"demo_start := _demo_start",
		"\texport demo_start",
	}
	for _, want := range mustContain {
		if !strings.Contains(image, want) {
			t.Errorf("image is missing %q:\n%s", want, image)
		}
	}

	body := strings.Index(image, "_demo_start:")
	alias := strings.Index(image, "_memcpy := ti._memcpy")
	rename := strings.Index(image, "demo_start := _demo_start")
	export := strings.Index(image, "\texport demo_start")

	if alias > body {
		t.Errorf("os aliases must precede the body")
	}
	if rename < body {
		t.Errorf("export aliases must follow the body, fasmg resolves := immediately")
	}
	if export < rename {
		t.Errorf("exports must follow their aliases")
	}
}

func TestEmitKeepsExportOrder(t *testing.T) {
	image := string(Emit(Options{
		Name:    "DEMO",
		Exports: []string{"first", "second", "third"},
		Body:    []byte("_first:\n_second:\n_third:\n"),
	}))

	first := strings.Index(image, "\texport first")
	second := strings.Index(image, "\texport second")
	third := strings.Index(image, "\texport third")

	if !(first < second && second < third) {
		t.Fatal("export order defines LibLoad ordinals and must be preserved")
	}
}
