package runstats

import (
	"math"

	"github.com/hivecommons/spektacular/internal/agentusage"
)

// Price is one model's price per million tokens of each type, in whatever
// currency the project's price table uses.
type Price struct {
	Input, Output, CacheRead, CacheWrite float64
}

// Prices are model prices keyed by the model name exactly as a report lists
// it. No prices are built in: a model with none has an unknown cost.
type Prices map[string]Price

// price is the cost of u tokens of model, and whether the model has a
// price.
func price(p Prices, model string, u agentusage.Usage) (cost float64, priced bool) {
	pr, ok := p[model]
	if !ok {
		return 0, false
	}
	return (float64(u.Input)*pr.Input + float64(u.Output)*pr.Output +
		float64(u.CacheRead)*pr.CacheRead + float64(u.CacheWrite)*pr.CacheWrite) / 1e6, true
}

// round6 is v rounded to 6 decimals.
func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }
