package doctor

import "sort"

// resolveVerdict orders findings, suppresses lower-priority findings that name
// the same container as a higher-priority finding, and returns the top one as
// the verdict. Empty input → healthy.
func resolveVerdict(in []Finding) (verdict *Finding, ordered []Finding, healthy bool) {
	if len(in) == 0 {
		return nil, nil, true
	}

	// Sort: Priority asc (1 = highest), Severity desc, Container asc.
	sort.SliceStable(in, func(i, j int) bool {
		a, b := in[i], in[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if severityRank(a.Severity) != severityRank(b.Severity) {
			return severityRank(a.Severity) > severityRank(b.Severity)
		}
		return a.Container < b.Container
	})

	ordered = suppressDuplicatesPerContainer(in)
	if len(ordered) == 0 {
		return nil, nil, true
	}
	v := ordered[0]
	return &v, ordered, false
}

// suppressDuplicatesPerContainer drops findings that name a container already
// claimed by a higher-priority finding. Findings without a container are
// always kept.
func suppressDuplicatesPerContainer(in []Finding) []Finding {
	seen := map[string]bool{}
	out := make([]Finding, 0, len(in))
	for _, f := range in {
		if f.Container == "" {
			out = append(out, f)
			continue
		}
		if seen[f.Container] {
			continue
		}
		seen[f.Container] = true
		out = append(out, f)
	}
	return out
}
