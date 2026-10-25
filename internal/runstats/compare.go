package runstats

import "math"

// Comparisons and summaries share one generic figure shape, so a later
// figure needs no new shape: each total and each step's figures are rows
// named by figure, in these orders.
var (
	totalFigures = []string{"wall_ms", "working_ms", "user_wait_ms", "helper_wait_ms", "command_ms", "commands", "helpers", "gap_ms"}
	stepFigures  = []string{"duration_ms", "entries", "commands", "command_ms", "helper_wait_ms", "user_wait_ms"}
)

// Figure is one figure across compared runs. Values, Delta and DeltaPct line
// up with the comparison's runs: a value is null for a run the figure does
// not apply to (a step it never entered); Delta is each value less the
// first run's, null when either is null; DeltaPct is Delta as a percentage
// of the first run's value, rounded to one decimal, null when that base is
// null or 0.
type Figure struct {
	Figure   string     `json:"figure"`
	Values   []*int64   `json:"values"`
	Delta    []*int64   `json:"delta"`
	DeltaPct []*float64 `json:"delta_pct"`
}

// StepFigures are one step's figures across runs.
type StepFigures struct {
	Step    string   `json:"step"`
	Figures []Figure `json:"figures"`
}

// Comparison puts runs side by side, step by step and in total, against the
// first run. Steps are keyed by name: the first run's steps in its order,
// then the steps of later runs not yet seen.
type Comparison struct {
	GapThresholdMS int64         `json:"gap_threshold_ms"`
	Runs           []RunSummary  `json:"runs"`
	Totals         []Figure      `json:"totals"`
	Steps          []StepFigures `json:"steps"`
}

// Compare compares runs, the first as the base. It is pure.
func Compare(runs []*Run, o Options) Comparison {
	t := buildTable(runs, o)
	c := Comparison{GapThresholdMS: ms(o.GapThreshold), Runs: t.runs, Totals: []Figure{}, Steps: []StepFigures{}}
	for i, name := range totalFigures {
		c.Totals = append(c.Totals, newFigure(name, t.totals[i]))
	}
	for _, step := range t.steps {
		sf := StepFigures{Step: step, Figures: []Figure{}}
		for i, name := range stepFigures {
			sf.Figures = append(sf.Figures, newFigure(name, t.stepValues[step][i]))
		}
		c.Steps = append(c.Steps, sf)
	}
	return c
}

// figureTable is every figure of every run, ready to compare or summarise:
// totals[f][r] and stepValues[step][f][r], null where a run never entered
// a step.
type figureTable struct {
	runs       []RunSummary
	totals     [][]*int64
	steps      []string
	stepValues map[string][][]*int64
}

func buildTable(runs []*Run, o Options) figureTable {
	t := figureTable{runs: []RunSummary{}, totals: make([][]*int64, len(totalFigures)), stepValues: map[string][][]*int64{}}
	for i := range t.totals {
		t.totals[i] = make([]*int64, len(runs))
	}
	for r, run := range runs {
		rep := BuildReport(run, o)
		t.runs = append(t.runs, rep.Run)
		for i, v := range totalValues(rep.Totals) {
			t.totals[i][r] = num(v)
		}
		for _, row := range rep.Steps {
			vals, ok := t.stepValues[row.Step]
			if !ok {
				t.steps = append(t.steps, row.Step)
				vals = make([][]*int64, len(stepFigures))
				for i := range vals {
					vals[i] = make([]*int64, len(runs))
				}
				t.stepValues[row.Step] = vals
			}
			for i, v := range stepRowValues(row) {
				vals[i][r] = num(v)
			}
		}
	}
	return t
}

// totalValues are a report's totals in totalFigures order.
func totalValues(t Totals) []int64 {
	return []int64{t.WallMS, t.WorkingMS, t.UserWaitMS, t.HelperWaitMS, t.CommandMS, int64(t.Commands), int64(t.Helpers), t.GapMS}
}

// stepRowValues are a step row's figures in stepFigures order.
func stepRowValues(s StepRow) []int64 {
	return []int64{s.DurationMS, int64(s.Entries), int64(s.Commands), s.CommandMS, s.HelperWaitMS, s.UserWaitMS}
}

func newFigure(name string, values []*int64) Figure {
	f := Figure{Figure: name, Values: values, Delta: make([]*int64, len(values)), DeltaPct: make([]*float64, len(values))}
	if len(values) == 0 || values[0] == nil {
		return f
	}
	base := *values[0]
	for i, v := range values {
		if v == nil {
			continue
		}
		d := *v - base
		f.Delta[i] = &d
		if base != 0 {
			p := math.Round(float64(d)/float64(base)*1000) / 10
			f.DeltaPct[i] = &p
		}
	}
	return f
}

func num(v int64) *int64 { return &v }
