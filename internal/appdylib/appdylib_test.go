package appdylib

import (
	"strings"
	"testing"
)

func TestTableLayout(t *testing.T) {
	table := string(Table("lwip", []string{"lwip_init", "lwip_stop"}))

	for _, want := range []string{
		".section .fn_exports,\"aR\",@progbits",
		"\tdb 'L','W','I','P','T','B'\n\td24 2\n\td24 _lwip_init\n\td24 _lwip_stop\n",
	} {
		if !strings.Contains(table, want) {
			t.Fatalf("table lacks %q:\n%s", want, table)
		}
	}
}

func TestStubOrdersBootstrapFirst(t *testing.T) {
	stub, err := Stub(StubOptions{
		Name:    "LWIP",
		Version: 1,
		App:     "lwIP",
		Prefix:  "lwip",
		Init:    "lwip_init",
		Imports: []string{"usb_Init"},
		Exports: []string{"lwip_init", "lwip_stop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(stub)

	start := strings.Index(text, "\texport lwip_start_with_crt")
	first := strings.Index(text, "\texport lwip_init")
	if start < 0 || first < start {
		t.Fatalf("bootstrap exports must come before the library exports:\n%s", text)
	}
	for _, want := range []string{
		"_fn_imports_table:\n\tdl 0\n\tdl 0\n\tdl 0\n\tdl usb_Init\n",
		"lwip_stop:\n\tjp __libify_unset\n",
		"__libify_export_count := 2",
		"db \"lwIP\", 0",
		"cp a, 'B'",
		"call lwip_init",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("stub lacks %q", want)
		}
	}
}

func TestStubKeepsListedBootstrapPositions(t *testing.T) {
	stub, err := Stub(StubOptions{
		Name:    "LWIP",
		App:     "lwIP",
		Prefix:  "lwip",
		Init:    "lwip_init",
		Exports: []string{"lwip_init", "lwip_start_with_crt", "lwip_get_start_errno", "lwip_get_start_errstring", "lwip_is_newer", "lwip_stop"},
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
	if strings.Join(exports, " ") != "lwip_init lwip_start_with_crt lwip_get_start_errno lwip_get_start_errstring lwip_is_newer lwip_stop" {
		t.Fatalf("export order = %v", exports)
	}
	if strings.Contains(text, "lwip_is_newer:\n\tjp") || !strings.Contains(text, "__libify_export_count := 2") {
		t.Fatalf("bootstrap functions must not get trampolines:\n%s", text)
	}
}

func TestStubNeedsExportedInit(t *testing.T) {
	_, err := Stub(StubOptions{Name: "LWIP", App: "lwIP", Prefix: "lwip", Init: "missing", Exports: []string{"lwip_stop"}})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("got %v", err)
	}
}

func TestLinkerScriptPinsTable(t *testing.T) {
	stock := ".header :\n{\n    KEEP (*(.header.icon))\n} >prgm\n\n.init :\n"

	script, err := LinkerScript([]byte(stock))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)

	pinned := strings.Index(text, ".fn_exports LOAD_ADDR + 0x40")
	if pinned < strings.Index(text, ">prgm") || pinned > strings.Index(text, ".init :") {
		t.Fatalf("export table not placed right after the header:\n%s", text)
	}

	if _, err := LinkerScript([]byte(".text : { }")); err == nil {
		t.Fatal("expected an error for a script without .header")
	}
}
