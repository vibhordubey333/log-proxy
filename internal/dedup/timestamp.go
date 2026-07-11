package dedup

import "regexp"

// timestampPrefix matches a leading "HH:mm:ss " prefix, e.g. "10:23:41 ".
// Assumed format based on Jenkins Timestamper/timestamps() step
// documentation — not verified against a real captured log, since
// anonymous access to ci.jenkins.io is currently blocked (see
// REFLECTION.md). If the real format differs, this is the one regex to change.

var timestampPrefix = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}\s+`)

// StripTimestamps removes a leading "HH:mm:ss " prefix from each line.
// Lines that don't match the pattern are returned unchanged — we don't
// assume every line has a timestamp (e.g. multi-line stack traces
// wrapped under a single timestamped line).
func StripTimestamps(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = timestampPrefix.ReplaceAllString(line, "")
	}
	return out
}
