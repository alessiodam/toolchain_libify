package srcfilter

import (
	"strings"
	"testing"
)

func transform(t *testing.T, raw string, opt Options) *Result {
	t.Helper()

	result, err := Transform([]byte(raw), opt)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	return result
}

func TestDropsDirectivesFasmgCannotRead(t *testing.T) {
	raw := "\t.section\t.text._foo,\"ax\",@progbits\n" +
		"\tpublic\t_foo\n" +
		"\tassume\tadl = 1\n" +
		"_foo:\n" +
		"\tret\n" +
		"\t.ident\t\"clang\"\n" +
		"\textern\t_bar\n" +
		"\tprivate\tBB0_1\n"

	got := string(transform(t, raw, Options{}).Bytes())

	for _, unwanted := range []string{".section", "public", "assume", ".ident", "extern", "private"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output still contains %q:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, "_foo:") || !strings.Contains(got, "\tret") {
		t.Errorf("code was dropped:\n%s", got)
	}
}

func TestStringLiteralsAreReEncoded(t *testing.T) {
	raw := "_msg:\n\tdb\t\"hi\x00\"\n_next:\n\tret\n"

	got := string(transform(t, raw, Options{}).Bytes())

	if strings.ContainsRune(got, 0) {
		t.Fatalf("a NUL survived, fasmg would stop reading the file here:\n%q", got)
	}
	if !strings.Contains(got, `db	"hi", 0`) {
		t.Errorf("unexpected encoding:\n%q", got)
	}
	if !strings.Contains(got, "_next:") {
		t.Errorf("content after the string was lost:\n%q", got)
	}
}

func TestStringLiteralWithNewlineAndQuoteBytes(t *testing.T) {
	raw := "_blob:\n\tdb\t\"a\nb\"c\x80\"\n_after:\n\tret\n"

	result := transform(t, raw, Options{})
	got := string(result.Bytes())

	if !result.Defined["_after"] {
		t.Fatalf("scanning stopped early, _after was never defined:\n%q", got)
	}
	for _, want := range []string{"10", "34", "128"} {
		if !strings.Contains(got, want) {
			t.Errorf("byte %s not encoded numerically:\n%q", want, got)
		}
	}
}

func TestEmptyStringLiteralEmitsNoBytes(t *testing.T) {
	raw := "_empty:\n\tdb\t\"\"\n"

	got := string(transform(t, raw, Options{}).Bytes())

	if strings.Contains(got, "db") {
		t.Errorf("an empty literal should not emit a db:\n%q", got)
	}
}

func TestRenameRewritesCodeButNotStrings(t *testing.T) {
	raw := "_go:\n\tcall\t_usb_Init\n\tdb\t\"_usb_Init\"\n"

	got := string(transform(t, raw, Options{Rename: map[string]string{"_usb_Init": "usb_Init"}}).Bytes())

	if !strings.Contains(got, "call\tusb_Init") {
		t.Errorf("call site was not renamed:\n%q", got)
	}
	if !strings.Contains(got, `"_usb_Init"`) {
		t.Errorf("string literal must not be rewritten:\n%q", got)
	}
}

func TestRenameLeavesLongerNamesAlone(t *testing.T) {
	raw := "_go:\n\tcall\t_usb_InitLater\n"

	got := string(transform(t, raw, Options{Rename: map[string]string{"_usb_Init": "usb_Init"}}).Bytes())

	if !strings.Contains(got, "_usb_InitLater") {
		t.Errorf("a longer identifier was clipped:\n%q", got)
	}
}

func TestCommentsAreNotScannedForSymbols(t *testing.T) {
	raw := "_foo:\t; @_ghost\n\tret\n"

	result := transform(t, raw, Options{})

	if _, ok := result.Refs["_ghost"]; ok {
		t.Errorf("identifier inside a comment was treated as a reference")
	}
}

func TestUnresolvedReportsWhatIsMissing(t *testing.T) {
	raw := "_here:\n\tcall\t_missing\n\tcall\t_here\n\tcall\t_provided\n"

	result := transform(t, raw, Options{})
	missing := result.Unresolved(map[string]bool{"_provided": true})

	if len(missing) != 1 || missing[0] != "_missing" {
		t.Fatalf("want [_missing], got %v", missing)
	}
	if result.Line("_missing") == 0 {
		t.Errorf("no line recorded for the missing symbol")
	}
}

func TestEmptyInputIsAnError(t *testing.T) {
	if _, err := Transform([]byte("\t.section\t.text\n"), Options{}); err == nil {
		t.Fatal("expected an error when nothing survives filtering")
	}
}
