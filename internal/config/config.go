// Package config loads an optional .gospect.yml from the scan directory, letting a repo set its
// default toggles once instead of repeating flags on every run. Explicit CLI flags always win.
// Filtering/suppression (exclude, ignore) lives in .gospectignore, not here.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config mirrors the behavioral flags worth setting per-repo. Bool fields are pointers so an omitted
// key stays "unset" (and never overrides a built-in default to false).
type Config struct {
	Pedantic    *bool  `yaml:"pedantic"`
	Staticcheck *bool  `yaml:"staticcheck"`
	Untested    *bool  `yaml:"untested"`
	Vuln        *bool  `yaml:"vuln"`
	FailOn      string `yaml:"fail-on"` // check gate: high|medium|low
}

// Load reads dir/.gospect.yml. It returns (nil, nil) when the file is absent — a missing config is
// not an error.
func Load(dir string) (*Config, error) {
	b, err := os.ReadFile(filepath.Join(dir, ".gospect.yml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf(".gospect.yml: %w", err)
	}
	return &c, nil
}

// ApplyBool sets *flagVal from the config value, but only when the flag wasn't passed explicitly
// (set) and the config key was present (cfgVal != nil) — so precedence is flag > file > default.
func ApplyBool(set map[string]bool, name string, flagVal *bool, cfgVal *bool) {
	if cfgVal != nil && !set[name] {
		*flagVal = *cfgVal
	}
}

// ApplyString is the string counterpart (empty config value = unset).
func ApplyString(set map[string]bool, name string, flagVal *string, cfgVal string) {
	if cfgVal != "" && !set[name] {
		*flagVal = cfgVal
	}
}
