package runstats

import "math"

// Comparisons and summaries share one generic figure shape, so a later
// figure needs no new shape: each total and each step's figures are rows
// named by figure, in these orders.
//
// Token use (spec 000078) appends tokenFigures to both lists, after the
// figures 000075 defined: a run whose token use is unknown, a null cost and
// a null pace give null there.
var (
	tokenFigures = []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "cost", "working_ms_per_1k_output"}
	totalFigures = append([]string{"wall_ms", "working_ms", "user_wait_ms", "helper_wait_ms", "command_ms", "commands", "helpers", "gap_ms"}, tokenFigures...)
	stepFigures  = append([]string{"duration_ms", "entries", "commands", "command_ms", "helper_wait_ms", "user_wait_ms"}, tokenFigures...)
)

// Figure is one figure across compared runs. Values, Delta and DeltaPct line
// up with the comparison's runs: a value is null for a run the figure does
// not apply to (a step it never entered); Delta is each value less the
// first run's, null when either is null; DeltaPct is Delta as a percentage
// of the first run's value, rounded to one decimal, null when that base is
// null or 0. Values are numbers: every figure but cost is a whole number,
// and encodes as one; a cost and its delta are rounded to 6 decimals.
type Figure struct {
	Figure   string     `json:"figure"`
	Values   []*float64 `json:"values"`
	Delta    []*float64 `json:"delta"`
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
	totals     [][]*float64
	steps      []string
	stepValues map[string][][]*float64
}

func buildTable(runs []*Run, o Options) figureTable {
	t := figureTable{runs: []RunSummary{}, totals: make([][]*float64, len(totalFigures)), stepValues: map[string][][]*float64{}}
	for i := range t.totals {
		t.totals[i] = make([]*float64, len(runs))
	}
	for r, run := range runs {
		rep := BuildReport(run, o)
		t.runs = append(t.runs, rep.Run)
		for i, v := range totalValues(rep.Totals, rep.Tokens.Total) {
			t.totals[i][r] = v
		}
		for _, row := range rep.Steps {
			vals, ok := t.stepValues[row.Step]
			if !ok {
				t.steps = append(t.steps, row.Step)
				vals = make([][]*float64, len(stepFigures))
				for i := range vals {
					vals[i] = make([]*float64, len(runs))
				}
				t.stepValues[row.Step] = vals
			}
			for i, v := range stepRowValues(row) {
				vals[i][r] = v
			}
		}
	}
	return t
}

// totalValues are a report's totals in totalFigures order, the token
// figures from the run's token total (null when unknown).
func totalValues(t Totals, tokens *TokenFigure) []*float64 {
	return append([]*float64{num(t.WallMS), num(t.WorkingMS), num(t.UserWaitMS), num(t.HelperWaitMS), num(t.CommandMS),
		num(int64(t.Commands)), num(int64(t.Helpers)), num(t.GapMS)}, tokenValues(tokens)...)
}

// stepRowValues are a step row's figures in stepFigures order.
func stepRowValues(s StepRow) []*float64 {
	return append([]*float64{num(s.DurationMS), num(int64(s.Entries)), num(int64(s.Commands)), num(s.CommandMS),
		num(s.HelperWaitMS), num(s.UserWaitMS)}, tokenValues(s.Tokens)...)
}

// tokenValues are a token figure's values in tokenFigures order: all null
// when token use is unknown; the cost and pace null when they are.
func tokenValues(f *TokenFigure) []*float64 {
	if f == nil {
		return make([]*float64, len(tokenFigures))
	}
	var pace *float64
	if f.WorkingMSPer1kOutput != nil {
		pace = num(*f.WorkingMSPer1kOutput)
	}
	var cost *float64
	if f.Cost != nil {
		c := *f.Cost
		cost = &c
	}
	return []*float64{num(f.Input), num(f.Output), num(f.CacheRead), num(f.CacheWrite), cost, pace}
}

func newFigure(name string, values []*float64) Figure {
	f := Figure{Figure: name, Values: values, Delta: make([]*float64, len(values)), DeltaPct: make([]*float64, len(values))}
	if len(values) == 0 || values[0] == nil {
		return f
	}
	base := *values[0]
	for i, v := range values {
		if v == nil {
			continue
		}
		d := *v - base
		if name == "cost" {
			d = round6(d)
		}
		f.Delta[i] = &d
		if base != 0 {
			p := math.Round(d/base*1000) / 10
			f.DeltaPct[i] = &p
		}
	}
	return f
}

func num(v int64) *float64 { f := float64(v); return &f }
