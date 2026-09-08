package libasm

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func OSRoutine(name string) string {
	switch {
	case strings.HasPrefix(name, "__"):
		return "ti._" + name[2:]
	case strings.HasPrefix(name, "_"):
		return "ti." + name
	default:
		return ""
	}
}

var Stubs = map[string]string{
	"__indcallhl": strings.Join([]string{
		"__indcallhl:",
		"\tjp\t(hl)",
	}, "\n"),
	"_atomic_load_32": strings.Join([]string{
		"_atomic_load_32:",
		"\tld\thl, 3",
		"\tadd\thl, sp",
		"\tld\thl, (hl)",
		"\tpush\thl",
		"\tpop\tiy",
		".retry:",
		"\tld\ta, (iy + 3)",
		"\tld\thl, (iy)",
		"\tld\te, a",
		"\tcp\ta, (iy + 3)",
		"\tjr\tnz, .retry",
		"\tret",
	}, "\n"),
}

func LoadOSSymbols(vendorDir string) (map[string]bool, error) {
	file, err := os.Open(filepath.Join(vendorDir, "ti84pceg.inc"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	names := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "?") {
			continue
		}
		index := strings.Index(line, ":=")
		if index < 0 {
			continue
		}
		names["ti."+strings.TrimSpace(line[1:index])] = true
	}

	return names, scanner.Err()
}

type Dependency struct {
	Name    string
	LibFile string
	Exports []string
}

func LoadDependency(libFile string) (*Dependency, error) {
	file, err := os.Open(libFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	dep := &Dependency{LibFile: libFile}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "library":
			dep.Name = strings.TrimSuffix(fields[1], ",")
		case "export":
			dep.Exports = append(dep.Exports, fields[1])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if dep.Name == "" {
		return nil, fmt.Errorf("%s: no library record", libFile)
	}

	return dep, nil
}

func (d *Dependency) Renames() map[string]string {
	out := make(map[string]string, len(d.Exports))
	for _, name := range d.Exports {
		out["_"+name] = name
	}
	return out
}

type Options struct {
	Name         string
	Version      int
	VendorDir    string
	Dependencies []*Dependency
	Exports      []string
	Aliases      map[string]string
	Stubs        []string
	Body         []byte
}

func forward(path string) string {
	return filepath.ToSlash(path)
}

func Emit(opt Options) []byte {
	var out bytes.Buffer

	fmt.Fprintf(&out, "include '%s'\n", forward(filepath.Join(opt.VendorDir, "library.inc")))
	fmt.Fprintf(&out, "include '%s'\n\n", forward(filepath.Join(opt.VendorDir, "include_library.inc")))
	fmt.Fprintf(&out, "library %s, %d\n\n", opt.Name, opt.Version)

	for _, dep := range opt.Dependencies {
		fmt.Fprintf(&out, "include_library '%s'\n", forward(dep.LibFile))
	}
	if len(opt.Dependencies) > 0 {
		out.WriteByte('\n')
	}

	out.WriteString("\tassume\tadl = 1\n\n")

	var helpers []string
	for name := range opt.Aliases {
		helpers = append(helpers, name)
	}
	sort.Strings(helpers)
	for _, name := range helpers {
		fmt.Fprintf(&out, "%s := %s\n", name, opt.Aliases[name])
	}
	out.WriteByte('\n')

	stubNames := append([]string(nil), opt.Stubs...)
	sort.Strings(stubNames)
	for _, name := range stubNames {
		out.WriteString(Stubs[name])
		out.WriteString("\n\n")
	}

	out.Write(opt.Body)

	out.WriteByte('\n')
	for _, name := range opt.Exports {
		fmt.Fprintf(&out, "%s := _%s\n", name, name)
	}
	out.WriteByte('\n')
	for _, name := range opt.Exports {
		fmt.Fprintf(&out, "\texport %s\n", name)
	}

	return out.Bytes()
}
