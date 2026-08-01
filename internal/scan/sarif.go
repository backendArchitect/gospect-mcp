package scan

import "path/filepath"

// SARIF renders the report as a SARIF 2.1.0 log (static-analysis interchange format). GitHub
// code-scanning ingests this and shows each finding as an inline annotation on the PR. Only the
// fields consumers rely on are populated — keep it lean.
//
// The return value is a plain map tree so it marshals to spec-compliant JSON without a schema dep.
func (r *Report) SARIF(version string) map[string]any {
	rules := map[string]bool{}
	var ruleList []map[string]any
	var results []map[string]any

	for _, f := range r.Findings {
		if !rules[f.Detector] {
			rules[f.Detector] = true
			ruleList = append(ruleList, map[string]any{
				"id":               f.Detector,
				"shortDescription": map[string]any{"text": f.Detector},
				"properties":       map[string]any{"category": f.Category},
			})
		}
		region := map[string]any{}
		if f.Line > 0 {
			region["startLine"] = f.Line
		}
		if f.Col > 0 {
			region["startColumn"] = f.Col
		}
		results = append(results, map[string]any{
			"ruleId":  f.Detector,
			"level":   sarifLevel(f.Severity),
			"message": map[string]any{"text": f.Message},
			"locations": []map[string]any{{
				"physicalLocation": map[string]any{
					"artifactLocation": map[string]any{"uri": filepath.ToSlash(r.relURI(f.File))},
					"region":           region,
				},
			}},
			"partialFingerprints": map[string]any{"gospect/v1": f.Fingerprint},
		})
	}

	return map[string]any{
		"$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		"version": "2.1.0",
		"runs": []map[string]any{{
			"tool": map[string]any{
				"driver": map[string]any{
					"name":           "gospect-mcp",
					"version":        version,
					"informationUri": "https://github.com/backendArchitect/gospect-mcp",
					"rules":          ruleList,
				},
			},
			"results": results,
		}},
	}
}

// sarifLevel maps a gospect severity to a SARIF result level.
func sarifLevel(sev string) string {
	switch sev {
	case "high":
		return "error"
	case "medium":
		return "warning"
	default:
		return "note"
	}
}

// relURI expresses a finding's file relative to the scan root for a portable SARIF uri, falling
// back to the original path when it can't be made relative.
func (r *Report) relURI(file string) string {
	if rel, err := filepath.Rel(r.Path, file); err == nil {
		return rel
	}
	return file
}
