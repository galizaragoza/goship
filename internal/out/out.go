// Package out prints the progress of a run as a sequence of titled sections,
// so each stage of a deploy stays apart from the next on the terminal instead
// of arriving as one unbroken stream of lines.
package out

import (
	"fmt"
	"os"
	"strings"
)

const (
	// step is one level of indentation. Everything a section reports is
	// written at least one level in, leaving the titles alone at the margin.
	step = "  "
	// width is how far the rule beside a section title is drawn.
	width = 68
)

// Section titles a stage of the run and sets it apart from the one before it.
// Everything printed until the next Section reads as belonging to it.
func Section(title string) {
	head := "── " + title + " "
	if pad := width - len([]rune(head)); pad > 0 {
		head += strings.Repeat("─", pad)
	}
	fmt.Printf("\n%s\n\n", head)
}

// Line reports something at depth levels below the current section title.
// Depth 0 is the section's own level; deeper values nest under the line above.
func Line(depth int, format string, args ...any) {
	fmt.Printf("%s%s\n", strings.Repeat(step, depth+1), fmt.Sprintf(format, args...))
}

// Item reports one thing done inside the current section.
func Item(format string, args ...any) {
	Line(0, format, args...)
}

// Result closes a section with what came of it, kept on a line of its own so
// it does not read as one more item.
func Result(format string, args ...any) {
	fmt.Println()
	Line(0, format, args...)
}

// Error reports on stderr, spaced away from whatever section was in progress
// so a failure is not lost among the lines that led to it.
func Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\n%s%s\n\n", step, fmt.Sprintf(format, args...))
}
