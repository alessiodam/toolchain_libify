package ceenv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Env struct {
	Prefix  string
	Clang   string
	Link    string
	Fasmg   string
	Verbose bool
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func Prefix(prefix string) (string, error) {
	if prefix == "" {
		out, err := exec.Command("cedev-config", "--prefix").Output()
		if err != nil {
			return "", fmt.Errorf("cedev-config --prefix: %w (is the CE toolchain on PATH?)", err)
		}
		prefix = strings.TrimSpace(string(out))
	}

	return filepath.Clean(prefix), nil
}

func Detect(prefix, fasmg string, verbose bool) (*Env, error) {
	prefix, err := Prefix(prefix)
	if err != nil {
		return nil, err
	}
	env := &Env{
		Prefix:  prefix,
		Clang:   filepath.Join(prefix, "bin", exeName("ez80-clang")),
		Link:    filepath.Join(prefix, "bin", exeName("ez80-link")),
		Fasmg:   fasmg,
		Verbose: verbose,
	}

	for _, tool := range []string{env.Clang, env.Link} {
		if _, err := os.Stat(tool); err != nil {
			return nil, fmt.Errorf("missing toolchain binary %s", tool)
		}
	}

	if env.Fasmg == "" {
		env.Fasmg = filepath.Join(prefix, "bin", exeName("fasmg"))
	}
	if _, err := os.Stat(env.Fasmg); err != nil {
		return nil, fmt.Errorf("missing fasmg binary %s, run get_fasmg.py or pass --fasmg", env.Fasmg)
	}

	return env, nil
}

func (e *Env) LibloadDir() string {
	return filepath.Join(e.Prefix, "lib", "libload")
}

func (e *Env) IncludeDir() string {
	return filepath.Join(e.Prefix, "include")
}

func (e *Env) VendorDir() string {
	return filepath.Join(e.Prefix, "meta", "libify")
}

func (e *Env) Run(name string, args ...string) error {
	if e.Verbose {
		fmt.Fprintln(os.Stderr, "+", name, strings.Join(args, " "))
	}

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return nil
}
