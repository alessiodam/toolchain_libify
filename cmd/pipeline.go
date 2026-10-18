package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path"
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
	include    []string
	define     []string
	exclude    []string
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

func (p *pipeline) excluded(rel string) (bool, error) {
	for _, pattern := range p.exclude {
		match, err := path.Match(pattern, rel)
		if err != nil {
			return false, err
		}
		if match {
			return true, nil
		}
	}
	return false, nil
}

func (p *pipeline) sources(ext string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(p.srcDir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(p.srcDir, file)
		if err != nil {
			return err
		}
		skip, err := p.excluded(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		switch {
		case skip && entry.IsDir():
			return filepath.SkipDir
		case !skip && !entry.IsDir() && filepath.Ext(file) == ext:
			files = append(files, file)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func (p *pipeline) compile() (string, error) {
	if err := os.MkdirAll(p.objDir, 0o755); err != nil {
		return "", err
	}

	sources, err := p.sources(".c")
	if err != nil {
		return "", err
	}
	if len(sources) == 0 {
		return "", fmt.Errorf("no C sources under %s", p.srcDir)
	}

	var bitcode []string
	for _, source := range sources {
		rel, err := filepath.Rel(p.srcDir, source)
		if err != nil {
			return "", err
		}
		out := filepath.Join(p.objDir, "src", rel+".bc")
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return "", err
		}
		args := []string{
			"-c", "-emit-llvm",
			"-nostdinc",
			"-isystem", p.env.IncludeDir(),
			"-I", p.headerDir,
			"-I", p.srcDir,
		}
		for _, dir := range p.include {
			args = append(args, "-I", dir)
		}
		for _, define := range p.define {
			args = append(args, "-D"+define)
		}
		args = append(args,
			p.optimize,
			"-Wall", "-Wextra",
			"-D__TICE__=1",
			"-fno-autolink",
			"-fno-addrsig",
			"-ffunction-sections",
			"-fdata-sections",
			source,
			"-o", out,
		)
		fmt.Fprintln(os.Stderr, "[cc]", filepath.ToSlash(rel))
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
