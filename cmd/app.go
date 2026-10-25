package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alessiodam/toolchain_libify/internal/appdylib"
)

const (
	defaultSplit     = 65200
	defaultInstaller = "https://github.com/CE-Programming/app_tools.git"
	installerProgram = "INSTALL.8xp"
)

type installerCache struct {
	URL         string `json:"url"`
	Commit      string `json:"commit"`
	PatchDigest string `json:"patch_digest"`
}

func installerCommitFromRefs(output, ref string) (string, error) {
	refs := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			refs[fields[1]] = fields[0]
		}
	}

	if ref == "" {
		if commit := refs["HEAD"]; commit != "" {
			return commit, nil
		}
		return "", fmt.Errorf("remote did not advertise HEAD")
	}

	candidates := []string{ref}
	if !strings.HasPrefix(ref, "refs/") {
		candidates = append(candidates,
			"refs/heads/"+ref,
			"refs/tags/"+ref+"^{}",
			"refs/tags/"+ref)
	} else if strings.HasPrefix(ref, "refs/tags/") {
		candidates = append([]string{ref + "^{}"}, candidates...)
	}
	for _, candidate := range candidates {
		if commit := refs[candidate]; commit != "" {
			return commit, nil
		}
	}
	return "", fmt.Errorf("ref %q was not advertised by remote", ref)
}

func resolveInstallerCommit(url, ref string) (string, error) {
	if (len(ref) == 40 || len(ref) == 64) && isHex(ref) {
		return strings.ToLower(ref), nil
	}
	args := []string{"ls-remote", url}
	if ref == "" {
		args = append(args, "HEAD")
	} else {
		args = append(args, ref)
		if !strings.HasPrefix(ref, "refs/") {
			args = append(args, "refs/heads/"+ref, "refs/tags/"+ref, "refs/tags/"+ref+"^{}")
		} else if strings.HasPrefix(ref, "refs/tags/") {
			args = append(args, ref+"^{}")
		}
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote: %w", err)
	}
	return installerCommitFromRefs(string(out), ref)
}

func isHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}

func installerPatchDigest(patches []string) (string, error) {
	hash := sha256.New()
	for _, patch := range patches {
		body, err := os.ReadFile(patch)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(filepath.ToSlash(patch)))
		hash.Write([]byte{0})
		hash.Write(body)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func installerCachePath(dir string) string {
	return filepath.Join(dir, ".git", "libify-cache.json")
}

func installerCacheMatches(dir string, want installerCache) bool {
	body, err := os.ReadFile(installerCachePath(dir))
	if err != nil {
		return false
	}
	var got installerCache
	return json.Unmarshal(body, &got) == nil && got == want
}

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
	url, ref := app.Installer, app.AppToolsRef
	if url == "" {
		url = defaultInstaller
	}

	if ref == "" {
		fmt.Fprintln(os.Stderr, "[installer]", url, "(default branch)")
	} else {
		fmt.Fprintln(os.Stderr, "[installer]", url, ref)
	}
	commit, err := resolveInstallerCommit(url, ref)
	if err != nil {
		return err
	}
	patchDigest, err := installerPatchDigest(app.Patches)
	if err != nil {
		return err
	}
	want := installerCache{URL: url, Commit: commit, PatchDigest: patchDigest}

	if !installerCacheMatches(dir, want) {
		staging := dir + ".tmp"
		if err := os.RemoveAll(staging); err != nil {
			return err
		}
		if err := p.env.Run("git", "clone", "--quiet", "--no-checkout", url, staging); err != nil {
			return err
		}
		if err := p.env.Run("git", "-C", staging, "checkout", "--quiet", "--detach", commit); err != nil {
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
		cacheBody, err := json.Marshal(want)
		if err != nil {
			return err
		}
		if err := os.WriteFile(installerCachePath(staging), cacheBody, 0o644); err != nil {
			return err
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.Rename(staging, dir); err != nil {
			return err
		}
	}

	installer := filepath.Join(dir, "installer")
	err = p.env.Run("make", "-B", "-C", installer,
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

	prefix := strings.ToLower(p.name)
	_, appExports := appdylib.Layout(prefix, exports)

	version, err := parseAppVersion(app.Version)
	if err != nil {
		return err
	}

	gen := filepath.Join(p.objDir, "libify")
	table := filepath.Join(gen, "functable.s")
	if err := writeIfChanged(table, appdylib.Table(p.name, appExports, app.Retain, version)); err != nil {
		return err
	}

	script := app.LinkerScript
	if script == "" {
		stock, err := os.ReadFile(filepath.Join(p.env.Prefix, "meta", "linker_script_app.ld"))
		if err != nil {
			return err
		}
		body, err := appdylib.LinkerScript(stock, version != "")
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
		Retain:       app.Retain,
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
