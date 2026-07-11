package dedup

import "fmt"

// CollapseConsecutive collapses runs of identical consecutive lines
// into a single line annotated with a count, e.g. three repeats of
// "Downloading junit-4.13.jar" become "Downloading junit-4.13.jar (x3)".
// Non-adjacent repeats (the same line reappearing later, with other
// lines in between) are intentionally left alone — see REFLECTION.md
// for why block-level/global dedup was scoped out.
func CollapseConsecutive(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}

	var out []string
	current := lines[0]
	count := 1

	flush := func() {
		if count > 1 {
			out = append(out, fmt.Sprintf("%s (x%d)", current, count))
		} else {
			out = append(out, current)
		}
	}

	for _, line := range lines[1:] {
		if line == current {
			count++
			continue
		}
		flush()
		current = line
		count = 1
	}
	flush()

	return out
}
