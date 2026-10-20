package appdylib

import (
	"strings"
	"testing"
)

func TestTableLayout(t *testing.T) {
	table := string(Table("example", []string{"example_init", "example_stop"}, false, ""))

	for _, want := range []string{
		".section .fn_exports,\"aR\",@progbits",
		".extern _example_init\n.extern _example_stop",
		"\tdb 'E','X','A','M','P','L','E','T','B'\n\td24 2\n\td24 _example_init\n\td24 _example_stop\n",
	} {
		if !strings.Contains(table, want) {
			t.Fatalf("table lacks %q:\n%s", want, table)
		}
	}
}

func TestStubOrdersBootstrapFirst(t *testing.T) {
	stub, err := Stub(StubOptions{
		Name:    "EXAMPLE",
		Version: 1,
		App:     "ExampleApp",
		Prefix:  "example",
		Init:    "example_init",
		Imports: []string{"usb_Init"},
		Exports: []string{"example_init", "example_stop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(stub)

	start := strings.Index(text, "\texport example_start_with_crt")
	first := strings.Index(text, "\texport example_init")
	if start < 0 || first < start {
		t.Fatalf("bootstrap exports must come before the library exports:\n%s", text)
	}
	for _, want := range []string{
		"_fn_imports_table:\n\tdl 0\n\tdl 0\n\tdl 0\n\tdl usb_Init\n",
		"example_stop:\n\tjp __libify_unset\n",
		"__libify_export_count := 2",
		"db \"ExampleApp\", 0",
		"cp a, 'B'",
		"call example_init",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("stub lacks %q", want)
		}
	}
}

func TestRetainRemovedExports(t *testing.T) {
	table := string(Table("example", []string{"example_init", "example_stop"}, true, ""))
	if !strings.Contains(table, ".weak _example_init\n.weak _example_stop") {
		t.Fatalf("table lacks weak exports:\n%s", table)
	}

	opt := StubOptions{Name: "EXAMPLE", App: "ExampleApp", Prefix: "example", Init: "example_init", Exports: []string{"example_init"}}
	plain, err := Stub(opt)
	if err != nil {
		t.Fatal(err)
	}
	opt.Retain = true
	retained, err := Stub(opt)
	if err != nil {
		t.Fatal(err)
	}
	skip := "jr z, .patch_next"
	if strings.Contains(string(plain), skip) || !strings.Contains(string(retained), skip) {
		t.Fatalf("zero slots must only be skipped when retaining removed exports")
	}
}

func TestStubKeepsListedBootstrapPositions(t *testing.T) {
	stub, err := Stub(StubOptions{
		Name:    "EXAMPLE",
		App:     "ExampleApp",
		Prefix:  "example",
		Init:    "example_init",
		Exports: []string{"example_init", "example_start_with_crt", "example_get_start_errno", "example_get_start_errstring", "example_is_newer", "example_stop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(stub)

	var exports []string
	for _, line := range strings.Split(text, "\n") {
		if name, ok := strings.CutPrefix(line, "\texport "); ok {
			exports = append(exports, name)
		}
	}
	if strings.Join(exports, " ") != "example_init example_start_with_crt example_get_start_errno example_get_start_errstring example_is_newer example_stop" {
		t.Fatalf("export order = %v", exports)
	}
	if strings.Contains(text, "example_is_newer:\n\tjp") || !strings.Contains(text, "__libify_export_count := 2") {
		t.Fatalf("bootstrap functions must not get trampolines:\n%s", text)
	}
}

func TestStubNeedsExportedInit(t *testing.T) {
	_, err := Stub(StubOptions{Name: "EXAMPLE", App: "ExampleApp", Prefix: "example", Init: "missing", Exports: []string{"example_stop"}})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("got %v", err)
	}
}

func TestLinkerScriptPinsTable(t *testing.T) {
	stock := ".header :\n{\n    KEEP (*(.header.icon))\n} >prgm\n\n.init :\n"

	script, err := LinkerScript([]byte(stock), false)
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)

	pinned := strings.Index(text, ".fn_exports LOAD_ADDR + 0x40")
	if pinned < strings.Index(text, ">prgm") || pinned > strings.Index(text, ".init :") {
		t.Fatalf("export table not placed right after the header:\n%s", text)
	}

	if _, err := LinkerScript([]byte(".text : { }"), false); err == nil {
		t.Fatal("expected an error for a script without .header")
	}
}

func TestTableVersion(t *testing.T) {
	table := string(Table("example", []string{"example_init"}, false, "2.1.3"))
	for _, want := range []string{
		".section .libify_version,\"aR\",@progbits",
		"_libify_version:\n\tdb \"2.1.3\",0\n",
	} {
		if !strings.Contains(table, want) {
			t.Fatalf("table lacks %q:\n%s", want, table)
		}
	}
	if strings.Contains(string(Table("example", []string{"example_init"}, false, "")), "libify_version") {
		t.Fatal("an unversioned table must not carry a version section")
	}
}

func TestLinkerScriptPinsVersion(t *testing.T) {
	stock := ".header :\n{\n    KEEP (*(.header.icon))\n} >prgm\n\n.init :\n"

	versioned, err := LinkerScript([]byte(stock), true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(versioned), ".libify_version LOAD_ADDR + 0x1000") {
		t.Fatalf("version string is not pinned:\n%s", versioned)
	}
	plain, err := LinkerScript([]byte(stock), false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "libify_version") {
		t.Fatalf("an unversioned script must not pin a version:\n%s", plain)
	}
}

func TestStubRestoresIXOnceOnInitFailure(t *testing.T) {
	stub, err := Stub(StubOptions{
		Name:    "EXAMPLE",
		App:     "ExampleApp",
		Prefix:  "example",
		Init:    "example_init",
		Exports: []string{"example_init"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// bootstrap.asm is checked out with CRLF line endings on Windows.
	text := strings.ReplaceAll(string(stub), "\r\n", "\n")
	want := "call example_init\n\tpop hl\n\tpop hl\n\n\tor a, a\n\tjr z, .init_failed\n\tpop ix\n\tld a, 1"
	if !strings.Contains(text, want) {
		t.Fatalf("the init result must be checked before the success path restores ix:\n%s", stub)
	}
}
