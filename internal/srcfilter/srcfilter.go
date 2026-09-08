package srcfilter

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
)

type Options struct {
	Rename map[string]string
}

type Result struct {
	Lines   [][]byte
	Defined map[string]bool
	Refs    map[string]int
}

var dropped = [][]byte{
	[]byte("\t.section"),
	[]byte("\t.ident"),
	[]byte("\t.addrsig"),
	[]byte("\tpublic\t"),
	[]byte("\textern\t"),
	[]byte("\tprivate\t"),
	[]byte("\tassume\t"),
}

var stringDB = []byte("\tdb\t\"")

func splitLines(raw []byte) [][]byte {
	var lines [][]byte

	for i := 0; i < len(raw); {
		if bytes.HasPrefix(raw[i:], stringDB) {
			end := bytes.Index(raw[i+len(stringDB):], []byte("\"\n"))
			if end >= 0 {
				stop := i + len(stringDB) + end + 1
				lines = append(lines, raw[i:stop])
				i = stop + 1
				continue
			}
		}

		end := bytes.IndexByte(raw[i:], '\n')
		if end < 0 {
			lines = append(lines, raw[i:])
			break
		}
		lines = append(lines, raw[i:i+end])
		i += end + 1
	}

	return lines
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '.' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentByte(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func mapIdentifiers(line []byte, f func(string) string) []byte {
	var out bytes.Buffer
	quoted := false

	for i := 0; i < len(line); {
		c := line[i]

		if c == '"' {
			quoted = !quoted
			out.WriteByte(c)
			i++
			continue
		}
		if quoted {
			out.WriteByte(c)
			i++
			continue
		}
		if c == ';' {
			out.Write(line[i:])
			break
		}
		if !isIdentStart(c) {
			out.WriteByte(c)
			i++
			continue
		}

		j := i
		for j < len(line) && isIdentByte(line[j]) {
			j++
		}
		out.WriteString(f(string(line[i:j])))
		i = j
	}

	return out.Bytes()
}

func sanitizeDB(payload []byte) []byte {
	var out bytes.Buffer
	out.WriteString("\tdb\t")

	var run bytes.Buffer
	first := true

	flushRun := func() {
		if run.Len() == 0 {
			return
		}
		if !first {
			out.WriteString(", ")
		}
		out.WriteByte('"')
		out.Write(run.Bytes())
		out.WriteByte('"')
		run.Reset()
		first = false
	}

	for _, b := range payload {
		if b >= 0x20 && b < 0x7f && b != '"' {
			run.WriteByte(b)
			continue
		}
		flushRun()
		if !first {
			out.WriteString(", ")
		}
		out.WriteString(strconv.Itoa(int(b)))
		first = false
	}
	flushRun()

	if first {
		return []byte("; empty string")
	}

	return out.Bytes()
}

func Transform(raw []byte, opt Options) (*Result, error) {
	res := &Result{
		Defined: map[string]bool{},
		Refs:    map[string]int{},
	}

	rename := func(name string) string {
		if to, ok := opt.Rename[name]; ok {
			return to
		}
		return name
	}

	for number, line := range splitLines(raw) {
		line = bytes.TrimRight(line, "\r")

		skip := false
		for _, prefix := range dropped {
			if bytes.HasPrefix(line, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		isString := bytes.HasPrefix(line, stringDB) && bytes.HasSuffix(line, []byte("\""))
		if isString {
			line = sanitizeDB(line[len(stringDB) : len(line)-1])
		}

		line = mapIdentifiers(line, rename)

		if end := bytes.IndexByte(line, ':'); end > 0 && isIdentStart(line[0]) {
			label := string(line[:end])
			if !bytes.ContainsAny(line[:end], " \t") {
				res.Defined[label] = true
			}
		}

		if !isString {
			mapIdentifiers(line, func(name string) string {
				if name[0] == '_' {
					if _, seen := res.Refs[name]; !seen {
						res.Refs[name] = number + 1
					}
				}
				return name
			})
		}

		res.Lines = append(res.Lines, line)
	}

	if len(res.Lines) == 0 {
		return nil, fmt.Errorf("no assembly left after filtering")
	}

	return res, nil
}

func (r *Result) Unresolved(provided map[string]bool) []string {
	var missing []string

	for name := range r.Refs {
		if r.Defined[name] || provided[name] {
			continue
		}
		missing = append(missing, name)
	}

	sort.Strings(missing)
	return missing
}

func (r *Result) Line(name string) int {
	return r.Refs[name]
}

func (r *Result) Bytes() []byte {
	return append(bytes.Join(r.Lines, []byte("\n")), '\n')
}
