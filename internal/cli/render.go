package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"text/tabwriter"
)

// outputFormat is the --output flag value.
type outputFormat string

const (
	formatText outputFormat = "text"
	formatJSON outputFormat = "json"
)

func (f outputFormat) valid() bool {
	return f == formatText || f == formatJSON
}

// renderJSON writes v as indented JSON. Used by every command's --output json.
func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// newTabWriter returns a tabwriter configured for aligned text tables.
func newTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
}

// fprintln is a thin helper so command code stays terse.
func fprintln(w io.Writer, args ...any) {
	fmt.Fprintln(w, args...)
}

// itoa is a terse int-to-string for table cells.
func itoa(n int) string { return strconv.Itoa(n) }

// sortedKeys returns a map's keys sorted, for deterministic text output.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
