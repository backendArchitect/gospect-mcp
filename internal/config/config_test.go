package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingIsNotError(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil || cfg != nil {
		t.Fatalf("missing config: got (%v, %v), want (nil, nil)", cfg, err)
	}
}

func TestLoad_ParsesKeys(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gospect.yml"), []byte("pedantic: true\nfail-on: medium\n"), 0o644)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Pedantic == nil || !*cfg.Pedantic {
		t.Errorf("pedantic = %v, want true", cfg.Pedantic)
	}
	if cfg.FailOn != "medium" {
		t.Errorf("fail-on = %q, want medium", cfg.FailOn)
	}
	if cfg.Staticcheck != nil {
		t.Errorf("staticcheck should be nil (unset), got %v", cfg.Staticcheck)
	}
}

func TestApplyBool_Precedence(t *testing.T) {
	tru := true
	// unset flag + config present -> config applies
	v := false
	ApplyBool(map[string]bool{}, "pedantic", &v, &tru)
	if !v {
		t.Error("config should apply when flag unset")
	}
	// explicit flag present -> config ignored
	v = false
	ApplyBool(map[string]bool{"pedantic": true}, "pedantic", &v, &tru)
	if v {
		t.Error("explicit flag must win over config")
	}
}
