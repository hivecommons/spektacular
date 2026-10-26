package runstats

import (
	"sort"
	"time"
)

// interval is a stretch of time from from to to.
type interval struct{ from, to time.Time }

// union sorts ivs and merges every overlapping or touching pair, so no
// moment is counted twice. Empty and reversed intervals are dropped.
func union(ivs []interval) []interval {
	var in []interval
	for _, iv := range ivs {
		if iv.to.After(iv.from) {
			in = append(in, iv)
		}
	}
	sort.Slice(in, func(i, j int) bool { return in[i].from.Before(in[j].from) })
	var out []interval
	for _, iv := range in {
		if n := len(out); n > 0 && !iv.from.After(out[n-1].to) {
			if iv.to.After(out[n-1].to) {
				out[n-1].to = iv.to
			}
			continue
		}
		out = append(out, iv)
	}
	return out
}

// clip is the parts of ivs inside window.
func clip(ivs []interval, window interval) []interval {
	var out []interval
	for _, iv := range ivs {
		from, to := iv.from, iv.to
		if from.Before(window.from) {
			from = window.from
		}
		if to.After(window.to) {
			to = window.to
		}
		if to.After(from) {
			out = append(out, interval{from, to})
		}
	}
	return out
}

// total is the summed length of ivs.
func total(ivs []interval) time.Duration {
	var d time.Duration
	for _, iv := range ivs {
		d += iv.to.Sub(iv.from)
	}
	return d
}
