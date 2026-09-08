package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alessiodam/toolchain_libify/internal/ceenv"
	"github.com/alessiodam/toolchain_libify/internal/libasm"
	"github.com/alessiodam/toolchain_libify/internal/srcfilter"
)

type pipeline struct {
	env *ceenv.Env

	name       string
	libVersion int
	srcDir     string
	headerDir  string
	vendorDir  string
	objDir     string
	binDir     string
	exportFile string
	dependency []string
	optimize   string
}

func (p *pipeline) dependencies() ([]*libasm.Dependency, error) {
	var deps []*libasm.Dependency

	for _, name := range p.dependency {
		path := name
		if filepath.Ext(path) != ".lib" {
			path = filepath.Join(p.env.LibloadDir(), name+".lib")
		}

		dep, err := libasm.LoadDependency(path)
		if err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}

	return deps, nil
}

func (p *pipeline) sources() ([]string, error) {
	entries, err := os.ReadDir(p.srcDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".c" {
			files = append(files, filepath.Join(p.srcDir, entry.Name()))
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		return nil, fmt.Errorf("no C sources under %s", p.srcDir)
	}
	return files, nil
}

func (p *pipeline) compile() (string, error) {
	if err := os.MkdirAll(p.objDir, 0o755); err != nil {
		return "", err
	}

	sources, err := p.sources()
	if err != nil {
		return "", err
	}

	var bitcode []string
	for _, source := range sources {
		out := filepath.Join(p.objDir, filepath.Base(source)+".bc")
		args := []string{
			"-c", "-emit-llvm",
			"-nostdinc",
			"-isystem", p.env.IncludeDir(),
			"-I", p.headerDir,
			"-I", p.srcDir,
			p.optimize,
			"-Wall", "-Wextra",
			"-D__TICE__=1",
			"-fno-autolink",
			"-fno-addrsig",
			"-ffunction-sections",
			"-fdata-sections",
			source,
			"-o", out,
		}
		fmt.Fprintln(os.Stderr, "[cc]", filepath.Base(source))
		if err := p.env.Run(p.env.Clang, args...); err != nil {
			return "", err
		}
		bitcode = append(bitcode, out)
	}

	merged := filepath.Join(p.objDir, "lto.bc")
	fmt.Fprintln(os.Stderr, "[lto]", filepath.Base(merged))
	if err := p.env.Run(p.env.Link, append(bitcode, "-o", merged)...); err != nil {
		return "", err
	}

	assembly := filepath.Join(p.objDir, "lto.src")
	fmt.Fprintln(os.Stderr, "[cc]", filepath.Base(assembly))
	err = p.env.Run(p.env.Clang,
		"-S", p.optimize,
		"-fno-autolink", "-fno-addrsig",
		"-ffunction-sections", "-fdata-sections",
		merged, "-o", assembly)
	if err != nil {
		return "", err
	}

	return assembly, nil
}

type resolution struct {
	aliases map[string]string
	stubs   []string
	missing []string
}

func (p *pipeline) resolve(result *srcfilter.Result, deps []*libasm.Dependency) (*resolution, error) {
	provided := map[string]bool{}
	for _, dep := range deps {
		for _, name := range dep.Exports {
			provided[name] = true
		}
	}

	osSymbols, err := libasm.LoadOSSymbols(p.vendorDir)
	if err != nil {
		return nil, err
	}

	res := &resolution{aliases: map[string]string{}}

	for _, name := range result.Unresolved(provided) {
		if _, ok := libasm.Stubs[name]; ok {
			res.stubs = append(res.stubs, name)
			continue
		}
		if target := libasm.OSRoutine(name); osSymbols[target] {
			res.aliases[name] = target
			continue
		}
		res.missing = append(res.missing, name)
	}

	return res, nil
}

func (p *pipeline) transform() ([]byte, []*libasm.Dependency, *resolution, error) {
	deps, err := p.dependencies()
	if err != nil {
		return nil, nil, nil, err
	}

	assembly, err := p.compile()
	if err != nil {
		return nil, nil, nil, err
	}

	raw, err := os.ReadFile(assembly)
	if err != nil {
		return nil, nil, nil, err
	}

	rename := map[string]string{}
	for _, dep := range deps {
		for from, to := range dep.Renames() {
			rename[from] = to
		}
	}

	result, err := srcfilter.Transform(raw, srcfilter.Options{Rename: rename})
	if err != nil {
		return nil, nil, nil, err
	}

	resolved, err := p.resolve(result, deps)
	if err != nil {
		return nil, nil, nil, err
	}

	if len(resolved.missing) > 0 {
		var report strings.Builder
		report.WriteString("unresolved symbols (a LibLoad library cannot link against libc):\n")
		for _, name := range resolved.missing {
			fmt.Fprintf(&report, "  %s\tfirst used at %s:%d\n", name, filepath.Base(assembly), result.Line(name))
		}
		return nil, nil, nil, fmt.Errorf("%s", report.String())
	}

	exports, err := readExports(p.exportFile)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := checkExports(exports, result); err != nil {
		return nil, nil, nil, err
	}

	image := libasm.Emit(libasm.Options{
		Name:         p.name,
		Version:      p.libVersion,
		VendorDir:    p.vendorDir,
		Dependencies: deps,
		Exports:      exports,
		Aliases:      resolved.aliases,
		Stubs:        resolved.stubs,
		Body:         result.Bytes(),
	})

	return image, deps, resolved, nil
}

func checkExports(exports []string, result *srcfilter.Result) error {
	var missing []string
	for _, name := range exports {
		if !result.Defined["_"+name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("exported but not defined by the library: %s", strings.Join(missing, ", "))
	}
	return nil
}
