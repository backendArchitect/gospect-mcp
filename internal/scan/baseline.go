package scan

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadBaseline reads a previously-saved gospect JSON report and returns the set of finding
// fingerprints it contains. Findings already in this set are "known" and can be hidden so a scan
// surfaces only what's NEW — the practical way to adopt gospect on a repo that already has hundreds
// of findings. A finding with no fingerprint (a very old report) is skipped.
func LoadBaseline(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("baseline %s is not a gospect JSON report: %w", path, err)
	}
	set := make(map[string]bool, len(rep.Findings))
	for _, f := range rep.Findings {
		if f.Fingerprint != "" {
			set[f.Fingerprint] = true
		}
	}
	return set, nil
}

// ApplyBaseline drops findings whose fingerprint is in the baseline set, keeping only new ones. It
// returns how many known findings were hidden.
func (r *Report) ApplyBaseline(baseline map[string]bool) (hidden int) {
	if len(baseline) == 0 {
		return 0
	}
	kept := r.Findings[:0]
	for _, f := range r.Findings {
		if baseline[f.Fingerprint] {
			hidden++
			continue
		}
		kept = append(kept, f)
	}
	r.Findings = kept
	r.recount()
	return hidden
}
