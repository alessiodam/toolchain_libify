package appdylib

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

const TableOffset = 0x40

//go:embed bootstrap.asm
var bootstrapSource string

var bootstrap = template.Must(template.New("bootstrap").Parse(bootstrapSource))

func magic(name string) []string {
	var chars []string
	for _, c := range strings.ToUpper(name) + "TB" {
		chars = append(chars, fmt.Sprintf("'%c'", c))
	}
	return chars
}

func Table(name string, exports []string) []byte {
	var out bytes.Buffer

	out.WriteString(".assume adl=1\n")
	out.WriteString(".section .fn_exports,\"aR\",@progbits\n")
	out.WriteString(".globl _fn_exports_table\n\n")
	for _, export := range exports {
		fmt.Fprintf(&out, ".extern _%s\n", export)
	}

	out.WriteString("\n_fn_exports_table:\n")
	fmt.Fprintf(&out, "\tdb %s\n", strings.Join(magic(name), ","))
	fmt.Fprintf(&out, "\td24 %d\n", len(exports))
	for _, export := range exports {
		fmt.Fprintf(&out, "\td24 _%s\n", export)
	}

	return out.Bytes()
}

type StubOptions struct {
	Name         string
	Version      int
	App          string
	Prefix       string
	Init         string
	VendorDir    string
	Dependencies []string
	Imports      []string
	Exports      []string
}

func (o StubOptions) Entries() []string {
	return []string{
		o.Prefix + "_start_with_crt",
		o.Prefix + "_get_start_errno",
		o.Prefix + "_get_start_errstring",
		o.Prefix + "_is_newer",
	}
}

func Stub(opt StubOptions) ([]byte, error) {
	if strings.ContainsAny(opt.App, "\"\n") {
		return nil, fmt.Errorf("application name %q cannot be quoted", opt.App)
	}

	found := false
	for _, export := range opt.Exports {
		if export == opt.Init {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("init function %s is not in the export list", opt.Init)
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "include '%s'\n", filepath.ToSlash(filepath.Join(opt.VendorDir, "library.inc")))
	fmt.Fprintf(&out, "include '%s'\n\n", filepath.ToSlash(filepath.Join(opt.VendorDir, "include_library.inc")))
	fmt.Fprintf(&out, "library %s, %d\n\n", opt.Name, opt.Version)
	for _, dep := range opt.Dependencies {
		fmt.Fprintf(&out, "include_library '%s'\n", filepath.ToSlash(dep))
	}
	out.WriteString("\n\tassume\tadl = 1\n\n")

	for _, name := range append(opt.Entries(), opt.Exports...) {
		fmt.Fprintf(&out, "\texport %s\n", name)
	}

	out.WriteString("\n_fn_imports_table:\n\tdl 0\n\tdl 0\n\tdl 0\n")
	for _, name := range opt.Imports {
		fmt.Fprintf(&out, "\tdl %s\n", name)
	}
	out.WriteString("_fn_imports_table_end:\n\n__libify_jp_table:\n")
	for _, name := range opt.Exports {
		fmt.Fprintf(&out, "%s:\n\tjp __libify_unset\n", name)
	}
	out.WriteByte('\n')

	err := bootstrap.Execute(&out, map[string]any{
		"Offset":  fmt.Sprintf("$%02X", TableOffset),
		"Exports": opt.Exports,
		"App":     opt.App,
		"Prefix":  opt.Prefix,
		"Init":    opt.Init,
		"Magic":   magic(opt.Name),
	})
	if err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

var headerEnd = "KEEP (*(.header.icon))"

func LinkerScript(stock []byte) ([]byte, error) {
	text := string(stock)

	at := strings.Index(text, headerEnd)
	if at < 0 {
		return nil, errors.New("linker script has no .header section to anchor the export table to")
	}
	end := strings.Index(text[at:], ">prgm")
	if end < 0 {
		return nil, errors.New("linker script .header section is not placed in prgm")
	}
	at += end + len(">prgm")

	descriptor := fmt.Sprintf(`

    ASSERT(. <= LOAD_ADDR + 0x%02X, "Error: application header exceeds the export table offset")

    .fn_exports LOAD_ADDR + 0x%02X :
    {
        KEEP(*(.fn_exports))
    } >prgm

    ASSERT(_fn_exports_table == LOAD_ADDR + 0x%02X, "Error: export table is not at its fixed offset")

    PROVIDE(____bss_low = ___bss_low);
    PROVIDE(____bss_len = ___bss_len);
    PROVIDE(____data_vma = ___data_vma);
    PROVIDE(____data_lma = ___data_lma);
    PROVIDE(____data_len = ___data_len);`, TableOffset, TableOffset, TableOffset)

	return []byte(text[:at] + descriptor + text[at:]), nil
}
