package dedup

import (
	"reflect"
	"testing"
)

func TestStripTimestamps(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "strips standard prefix",
			in:   []string{"10:23:41 Started by user anonymous"},
			want: []string{"Started by user anonymous"},
		},
		{
			name: "leaves non-matching lines untouched",
			in:   []string{"not a timestamp line"},
			want: []string{"not a timestamp line"},
		},
		{
			name: "handles empty input",
			in:   []string{},
			want: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripTimestamps(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCollapseConsecutive(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "collapses a run of duplicates",
			in:   []string{"a", "dup", "dup", "dup", "b"},
			want: []string{"a", "dup (x3)", "b"},
		},
		{
			name: "leaves non-adjacent duplicates alone",
			in:   []string{"dup", "b", "dup"},
			want: []string{"dup", "b", "dup"},
		},
		{
			name: "no duplicates at all",
			in:   []string{"a", "b", "c"},
			want: []string{"a", "b", "c"},
		},
		{
			name: "empty input",
			in:   []string{},
			want: []string{},
		},
		{
			name: "single line",
			in:   []string{"only"},
			want: []string{"only"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CollapseConsecutive(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
