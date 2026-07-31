package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndFind(t *testing.T) {
	dir := t.TempDir()
	// vendored spec should be ignored by Find
	_ = os.MkdirAll(filepath.Join(dir, "vendor"), 0o755)
	os.WriteFile(filepath.Join(dir, "vendor", "swagger.json"), []byte(`{"paths":{"/x":{"get":{}}}}`), 0o644)

	jsonSpec := filepath.Join(dir, "swagger.json")
	os.WriteFile(jsonSpec, []byte(`{"paths":{"/reservations/{id}":{"get":{},"delete":{}},"/health":{"get":{}}}}`), 0o644)
	yamlSpec := filepath.Join(dir, "openapi.yaml")
	os.WriteFile(yamlSpec, []byte("paths:\n  /items:\n    post: {}\n    parameters: []\n"), 0o644)

	files, err := Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 { // vendor one excluded
		t.Fatalf("Find: want 2 specs, got %d: %v", len(files), files)
	}

	eps, err := Load(jsonSpec)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 3 { // GET+DELETE /reservations/{id}, GET /health
		t.Fatalf("json Load: want 3 endpoints, got %d: %+v", len(eps), eps)
	}

	yeps, err := Load(yamlSpec)
	if err != nil {
		t.Fatal(err)
	}
	// only POST /items — "parameters" is not a method
	if len(yeps) != 1 || yeps[0].Method != "POST" || yeps[0].Path != "/items" {
		t.Fatalf("yaml Load: unexpected %+v", yeps)
	}
}
