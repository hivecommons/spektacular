package runstats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// iv is the interval from minute a to minute b.
func iv(a, b float64) interval { return interval{at(a), at(b)} }

func TestUnion(t *testing.T) {
	cases := []struct {
		name string
		in   []interval
		want []interval
	}{
		{"none", nil, nil},
		{"disjoint, unsorted", []interval{iv(5, 6), iv(0, 1)}, []interval{iv(0, 1), iv(5, 6)}},
		{"overlap", []interval{iv(0, 4), iv(2, 6)}, []interval{iv(0, 6)}},
		{"nested", []interval{iv(0, 10), iv(2, 3)}, []interval{iv(0, 10)}},
		{"nested listed first", []interval{iv(2, 3), iv(0, 10)}, []interval{iv(0, 10)}},
		{"touching edges", []interval{iv(2, 4), iv(0, 2)}, []interval{iv(0, 4)}},
		{"chain", []interval{iv(0, 2), iv(1, 3), iv(3, 5), iv(7, 8)}, []interval{iv(0, 5), iv(7, 8)}},
		{"empty and reversed dropped", []interval{iv(3, 3), iv(5, 4)}, nil},
		{"empty inside a gap dropped", []interval{iv(0, 1), iv(2, 2), iv(3, 4)}, []interval{iv(0, 1), iv(3, 4)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, union(c.in))
		})
	}
}

func TestClip(t *testing.T) {
	window := iv(10, 20)
	cases := []struct {
		name string
		in   []interval
		want []interval
	}{
		{"none", nil, nil},
		{"inside", []interval{iv(12, 15)}, []interval{iv(12, 15)}},
		{"straddles the start", []interval{iv(8, 12)}, []interval{iv(10, 12)}},
		{"straddles the end", []interval{iv(18, 25)}, []interval{iv(18, 20)}},
		{"covers the window", []interval{iv(0, 30)}, []interval{iv(10, 20)}},
		{"outside", []interval{iv(0, 5), iv(25, 30)}, nil},
		{"touching edges", []interval{iv(5, 10), iv(20, 22)}, nil},
		{"several", []interval{iv(9, 11), iv(14, 15), iv(19, 21)}, []interval{iv(10, 11), iv(14, 15), iv(19, 20)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, clip(c.in, window))
		})
	}
}

func TestTotal(t *testing.T) {
	cases := []struct {
		name string
		in   []interval
		want time.Duration
	}{
		{"none", nil, 0},
		{"one", []interval{iv(0, 1.5)}, 90 * time.Second},
		{"disjoint", []interval{iv(0, 1), iv(2, 4)}, 3 * time.Minute},
		{"overlap counted once after union", union([]interval{iv(0, 4), iv(2, 6)}), 6 * time.Minute},
		{"nested counted once after union", union([]interval{iv(0, 10), iv(2, 3)}), 10 * time.Minute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, total(c.in))
		})
	}
}
