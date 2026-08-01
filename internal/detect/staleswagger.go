package detect

import (
	"context"
	"strings"

	"github.com/backendArchitect/gospect-mcp/internal/graph"
	"github.com/backendArchitect/gospect-mcp/internal/spec"
)

// RunStaleSwagger finds OpenAPI/Swagger specs under dir and reports documented endpoints that
// have no matching registered route (a likely stale doc). Requires a configured graph for the
// route set. If no spec is present it returns nothing.
//
// Path matching is heuristic: path params ({id}/:id) are normalized and base-path differences
// are tolerated via suffix matching, so this is report-first (a human/agent confirms).
func RunStaleSwagger(ctx context.Context, g graph.Graph, dir string) ([]Finding, error) {
	specFiles, err := spec.Find(dir)
	if err != nil {
		return nil, err
	}
	if len(specFiles) == 0 {
		return nil, nil
	}

	routes, err := g.Routes(ctx)
	if err != nil {
		return nil, err
	}
	routeNorms := make([]string, 0, len(routes))
	exact := make(map[string]bool, len(routes))
	for _, r := range routes {
		n := normalizePath(r.Path)
		routeNorms = append(routeNorms, n)
		exact[strings.ToUpper(r.Method)+" "+n] = true
	}

	var findings []Finding
	for _, sf := range specFiles {
		eps, err := spec.Load(sf)
		if err != nil {
			continue // a malformed/partial spec shouldn't abort the whole scan
		}
		for _, ep := range eps {
			if routeExists(ep.Method, normalizePath(ep.Path), exact, routeNorms) {
				continue
			}
			findings = append(findings, Finding{
				Category:   "stale-doc",
				Detector:   "swagger-drift",
				Severity:   "medium",
				Confidence: "medium",
				File:       sf,
				Message:    "documented endpoint " + ep.Method + " " + ep.Path + " has no matching route (stale doc?)",
			})
		}
	}
	return findings, nil
}

// routeExists reports whether a documented (method, normalized-path) has a plausible route.
// Exact method+path wins; otherwise a suffix match on the path (ignoring method) tolerates
// base-path prefixes that routers add but specs omit (and vice versa).
func routeExists(method, docPath string, exact map[string]bool, routeNorms []string) bool {
	if exact[strings.ToUpper(method)+" "+docPath] {
		return true
	}
	for _, rn := range routeNorms {
		if rn == docPath || strings.HasSuffix(rn, docPath) || strings.HasSuffix(docPath, rn) {
			return true
		}
	}
	return false
}

// normalizePath lowercases nothing but collapses path params to "*", trims a trailing slash, and
// ensures a leading slash, so "/x/{id}/" and "/x/:id" both become "/x/*".
func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if strings.HasPrefix(s, "{") || strings.HasPrefix(s, ":") {
			segs[i] = "*"
		}
	}
	n := strings.Join(segs, "/")
	if len(n) > 1 {
		n = strings.TrimRight(n, "/")
	}
	if !strings.HasPrefix(n, "/") {
		n = "/" + n
	}
	return n
}
