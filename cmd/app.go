package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alessiodam/toolchain_libify/internal/appdylib"
)

const (
	defaultSplit        = 65200
	defaultInstaller    = "https://github.com/CE-Programming/app_tools.git"
	defaultInstallerRef = "3409a9d859c80f258e97e0efd46a0527b8469d96"
	installerProgram    = "INSTALL.8xp"
)

func writeIfChanged(path string, body []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, body) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func makeList(files []string) string {
	var out []string
	for _, file := range files {
		out = append(out, filepath.ToSlash(file))
	}
	return strings.Join(out, " \\\n\t")
}

func (p *pipeline) appMakefile(table, script string) ([]byte, error) {
	csources, err := p.sources(".c")
	if err != nil {
		return nil, err
	}
	asmsources, err := p.sources(".s")
	if err != nil {
		return nil, err
	}
	asmsources = append(asmsources, table)

	libs, err := filepath.Glob(filepath.Join(p.env.LibloadDir(), "*.lib"))
	if err != nil {
		return nil, err
	}
	var linked []string
	for _, lib := range libs {
		if !strings.EqualFold(filepath.Base(lib), p.name+".lib") {
			linked = append(linked, lib)
		}
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "NAME = %s\n", app.Name)
	out.WriteString("APPLICATION = YES\n")
	fmt.Fprintf(&out, "SRCDIR = %s\n", filepath.ToSlash(p.srcDir))
	fmt.Fprintf(&out, "OBJDIR = %s\n", filepath.ToSlash(p.objDir))
	fmt.Fprintf(&out, "BINDIR = %s\n", filepath.ToSlash(p.binDir))
	fmt.Fprintf(&out, "CEDEV_TOOLCHAIN = %s\n", filepath.ToSlash(p.env.Prefix))

	var keys []string
	for key := range app.Make {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&out, "%s = %s\n", key, app.Make[key])
	}

	fmt.Fprintf(&out, "LIBLOAD_LIBS = %s\n", makeList(linked))
	fmt.Fprintf(&out, "override LINKER_SCRIPT = %s\n", filepath.ToSlash(script))
	fmt.Fprintf(&out, "override CSOURCES = %s\n", makeList(csources))
	fmt.Fprintf(&out, "override ASMSOURCES = %s\n", makeList(asmsources))
	fmt.Fprintf(&out, "\ninclude %s\n", filepath.ToSlash(filepath.Join(p.env.Prefix, "meta", "makefile.mk")))

	return out.Bytes(), nil
}

func (p *pipeline) installer(split int) error {
	dir := filepath.Join(p.objDir, "libify", "app_tools")

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		url, ref := app.Installer, app.InstallerRef
		if url == "" {
			url = defaultInstaller
		}
		if ref == "" {
			ref = defaultInstallerRef
		}

		staging := dir + ".tmp"
		if err := os.RemoveAll(staging); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "[installer]", url, ref)
		if err := p.env.Run("git", "clone", "--quiet", url, staging); err != nil {
			return err
		}
		if err := p.env.Run("git", "-C", staging, "checkout", "--quiet", ref); err != nil {
			return err
		}
		for _, patch := range app.Patches {
			abs, err := filepath.Abs(patch)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "[installer] apply", patch)
			if err := p.env.Run("git", "-C", staging, "apply", abs); err != nil {
				return err
			}
		}
		if err := os.Rename(staging, dir); err != nil {
			return err
		}
	}

	installer := filepath.Join(dir, "installer")
	err := p.env.Run("make", "-B", "-C", installer,
		"APPVAR_PREFIX="+p.name,
		"APPVAR_SPLIT_SIZE="+strconv.Itoa(split),
		"CEDEV_TOOLCHAIN="+filepath.ToSlash(p.env.Prefix))
	if err != nil {
		return err
	}
	return copyFile(filepath.Join(installer, "bin", installerProgram), filepath.Join(p.binDir, installerProgram))
}

func buildApp(p *pipeline) error {
	exports, err := readExports(p.exportFile)
	if err != nil {
		return err
	}

	deps, err := p.dependencies()
	if err != nil {
		return err
	}
	var depFiles []string
	for _, dep := range deps {
		depFiles = append(depFiles, dep.LibFile)
	}

	gen := filepath.Join(p.objDir, "libify")
	table := filepath.Join(gen, "functable.s")
	if err := writeIfChanged(table, appdylib.Table(p.name, exports)); err != nil {
		return err
	}

	script := app.LinkerScript
	if script == "" {
		stock, err := os.ReadFile(filepath.Join(p.env.Prefix, "meta", "linker_script_app.ld"))
		if err != nil {
			return err
		}
		body, err := appdylib.LinkerScript(stock)
		if err != nil {
			return err
		}
		script = filepath.Join(gen, "linker_script.ld")
		if err := writeIfChanged(script, body); err != nil {
			return err
		}
	}

	makefile, err := p.appMakefile(table, script)
	if err != nil {
		return err
	}
	appMk := filepath.Join(gen, "app.mk")
	if err := writeIfChanged(appMk, makefile); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "[app]", app.Name)
	if err := p.env.Run("make", "-f", appMk); err != nil {
		return err
	}

	split := app.Split
	if split == 0 {
		split = defaultSplit
	}
	old, err := filepath.Glob(filepath.Join(p.binDir, p.name+".*.8xv"))
	if err != nil {
		return err
	}
	for _, file := range old {
		if err := os.Remove(file); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stderr, "[split]", p.name)
	err = p.env.Run(p.env.Convbin(),
		"--iformat", "8ek", "--input", filepath.Join(p.binDir, app.Name+".8ek"),
		"--oformat", "8xv-split", "--maxvarsize", strconv.Itoa(split),
		"--name", p.name, "--output", filepath.Join(p.binDir, p.name+".8xv"))
	if err != nil {
		return err
	}

	if err := p.installer(split); err != nil {
		return err
	}

	prefix := strings.ToLower(p.name)
	stub, err := appdylib.Stub(appdylib.StubOptions{
		Name:         p.name,
		Version:      p.libVersion,
		App:          app.Name,
		Prefix:       prefix,
		Init:         app.Init,
		VendorDir:    p.vendorDir,
		Dependencies: depFiles,
		Imports:      app.Imports,
		Exports:      exports,
	})
	if err != nil {
		return err
	}
	stubAsm := filepath.Join(gen, p.name+".asm")
	if err := writeIfChanged(stubAsm, stub); err != nil {
		return err
	}
	appvar := filepath.Join(p.binDir, p.name+".8xv")
	if err := p.env.Run(p.env.Fasmg, stubAsm, appvar); err != nil {
		return err
	}

	files, err := releaseFiles()
	if err != nil {
		return err
	}
	for _, file := range files {
		fmt.Fprintf(os.Stderr, "[success] %s\n", file)
	}
	return nil
}
