package runstats

import "sort"

// Stat is one figure summarised over runs: how many runs it applies to (for
// a step, the runs that entered it), their median, and their 90th
// percentile. Median and P90 are null when no run applies.
type Stat struct {
	Figure string   `json:"figure"`
	Count  int      `json:"count"`
	Median *float64 `json:"median"`
	P90    *float64 `json:"p90"`
}

// StepStats are one step's figures summarised over runs.
type StepStats struct {
	Step    string `json:"step"`
	Figures []Stat `json:"figures"`
}

// Summary summarises many runs, with the same shape as a Comparison: each
// figure gives the count, median and 90th percentile instead of the runs'
// values.
type Summary struct {
	GapThresholdMS int64        `json:"gap_threshold_ms"`
	Runs           []RunSummary `json:"runs"`
	Totals         []Stat       `json:"totals"`
	Steps          []StepStats  `json:"steps"`
}

// Summarise summarises runs. The median of an even count is the mean of the
// two middle values; the 90th percentile is the nearest-rank value, so it is
// always one the runs gave. It is pure.
func Summarise(runs []*Run, o Options) Summary {
	t := buildTable(runs, o)
	s := Summary{GapThresholdMS: ms(o.GapThreshold), Runs: t.runs, Totals: []Stat{}, Steps: []StepStats{}}
	for i, name := range totalFigures {
		s.Totals = append(s.Totals, newStat(name, t.totals[i]))
	}
	for _, step := range t.steps {
		ss := StepStats{Step: step, Figures: []Stat{}}
		for i, name := range stepFigures {
			ss.Figures = append(ss.Figures, newStat(name, t.stepValues[step][i]))
		}
		s.Steps = append(s.Steps, ss)
	}
	return s
}

func newStat(name string, values []*float64) Stat {
	var vs []float64
	for _, v := range values {
		if v != nil {
			vs = append(vs, *v)
		}
	}
	st := Stat{Figure: name, Count: len(vs)}
	if len(vs) == 0 {
		return st
	}
	sort.Float64s(vs)
	n := len(vs)
	median := vs[n/2]
	if n%2 == 0 {
		median = (vs[n/2-1] + vs[n/2]) / 2
	}
	p90 := vs[(9*n+9)/10-1] // the ceil(0.9 n)-th smallest
	st.Median, st.P90 = &median, &p90
	return st
}
