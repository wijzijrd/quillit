package migrate

import (
	"sort"
	"strings"
)

// QuickViewCard converts a legacy entry's quick_view_data fields into a
// single ":::card <facet>" block, one bold-label line per non-empty field
// (spec §5 point 4: one fixed rule, applied uniformly). Returns "" when no
// field has a value, so callers can omit the block entirely.
func QuickViewCard(facet string, fields map[string]string) string {
	labels := make([]string, 0, len(fields))
	for label, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return ""
	}
	sort.Strings(labels)

	var b strings.Builder
	b.WriteString(":::card ")
	b.WriteString(facet)
	for _, label := range labels {
		b.WriteString("\n**")
		b.WriteString(label)
		b.WriteString(":** ")
		b.WriteString(fields[label])
	}
	b.WriteString("\n:::")
	return b.String()
}
