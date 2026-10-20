package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const manifestFile = "ce-libify.toml"

type appBuild struct {
	Name         string            `toml:"name"`
	Init         string            `toml:"init"`
	Imports      []string          `toml:"imports"`
	LinkerScript string            `toml:"linker_script"`
	Split        int               `toml:"split"`
	Installer    string            `toml:"installer"`
	InstallerRef string            `toml:"installer_ref"`
	Patches      []string          `toml:"installer_patches"`
	Make         map[string]string `toml:"make"`
	Retain       bool              `toml:"retain_removed"`
}

type step struct {
	Run []string `toml:"run"`
}

var (
	app   *appBuild
	steps []step
)

func knownFlag(name string) bool {
	if rootCmd.PersistentFlags().Lookup(name) != nil {
		return true
	}
	for _, command := range rootCmd.Commands() {
		if command.Flags().Lookup(name) != nil {
			return true
		}
	}
	return false
}

func applyManifest(flags *pflag.FlagSet, path string) error {
	app, steps = nil, nil

	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	var values map[string]any
	if err := toml.Unmarshal(body, &values); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	var parsed struct {
		App  *appBuild `toml:"app"`
		Step []step    `toml:"step"`
	}
	if err := toml.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if parsed.App != nil && (parsed.App.Name == "" || parsed.App.Init == "") {
		return fmt.Errorf("%s: [app] needs name and init", path)
	}
	for _, s := range parsed.Step {
		if len(s.Run) == 0 {
			return fmt.Errorf("%s: [[step]] needs run", path)
		}
	}
	app, steps = parsed.App, parsed.Step
	delete(values, "app")
	delete(values, "step")

	var keys []string
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if !knownFlag(key) {
			return fmt.Errorf("%s: unknown key %q", path, key)
		}
		flag := flags.Lookup(key)
		if flag == nil || flag.Changed {
			continue
		}

		items, isList := values[key].([]any)
		if !isList {
			items = []any{values[key]}
		}
		for _, item := range items {
			if err := flags.Set(key, fmt.Sprint(item)); err != nil {
				return fmt.Errorf("%s: %s: %w", path, key, err)
			}
		}
	}

	return nil
}

func rejectApp(cmd *cobra.Command) error {
	if app != nil {
		return fmt.Errorf("%s builds an application library, %s is not available", manifestFile, cmd.Name())
	}
	return nil
}
