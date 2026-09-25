package cli

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// WriteTable writes a header row and data rows as an aligned table using
// stdlib text/tabwriter only. Cell values are sanitized with oneLine.
func WriteTable(out io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(out, 0, 8, 2, ' ', 0)
	writeTableRow(tw, headers)
	for _, row := range rows {
		writeTableRow(tw, row)
	}
	_ = tw.Flush()
}

// WriteEmpty writes a clear empty-list message on stdout-style writers.
func WriteEmpty(out io.Writer, msg string) {
	fmt.Fprintln(out, msg)
}

func writeTableRow(w io.Writer, cols []string) {
	for i, col := range cols {
		if i > 0 {
			_, _ = io.WriteString(w, "\t")
		}
		_, _ = io.WriteString(w, oneLine(col))
	}
	_, _ = io.WriteString(w, "\n")
}
