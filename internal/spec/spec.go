// Package spec discovers and parses OpenAPI/Swagger spec files (JSON or YAML) into a flat list
// of documented endpoints, for the stale-swagger detector.
package spec

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"

	"os"

	"gopkg.in/yaml.v3"
)

// Endpoint is one documented API operation.
type Endpoint struct {
	Method string
	Path   string
}

var specFileNames = map[string]bool{
	"swagger.json": true, "swagger.yaml": true, "swagger.yml": true,
	"openapi.json": true, "openapi.yaml": true, "openapi.yml": true,
}

var httpMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "HEAD": true, "OPTIONS": true,
}

// Find returns OpenAPI/Swagger spec files under dir, skipping vendor / node_modules / hidden dirs.
func Find(dir string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than aborting the walk
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" || (len(name) > 1 && strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if specFileNames[strings.ToLower(d.Name())] {
			found = append(found, path)
		}
		return nil
	})
	return found, err
}

// Load parses a spec file (JSON or YAML) and returns its documented endpoints.
func Load(path string) ([]Endpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Paths map[string]map[string]any `json:"paths" yaml:"paths"`
	}
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		err = json.Unmarshal(data, &doc)
	} else {
		err = yaml.Unmarshal(data, &doc)
	}
	if err != nil {
		return nil, err
	}

	var eps []Endpoint
	for p, ops := range doc.Paths {
		for method := range ops {
			m := strings.ToUpper(method)
			if httpMethods[m] { // skip non-method keys like "parameters"
				eps = append(eps, Endpoint{Method: m, Path: p})
			}
		}
	}
	return eps, nil
}
