package runstats

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrice(t *testing.T) {
	c, ok := price(tokPrices, "opus", use(1000, 2000, 30000, 4000))
	require.True(t, ok)
	// (1000*15 + 2000*75 + 30000*1.5 + 4000*18.75)/1e6
	require.InDelta(t, 0.285, c, 1e-12)

	c, ok = price(tokPrices, "sonnet", use(1000, 2000, 30000, 4000))
	require.False(t, ok)
	require.Zero(t, c)

	c, ok = price(nil, "opus", use(1, 1, 1, 1))
	require.False(t, ok)
	require.Zero(t, c)
}

func TestRound6(t *testing.T) {
	require.Equal(t, 1.234568, round6(1.23456789))
	require.Equal(t, 0.3, round6(0.1+0.2))
	require.Equal(t, 0.0, round6(0.0000004))
	require.Equal(t, 2.0, round6(2))
}

func TestFigurePricing(t *testing.T) {
	two := func() usageByModel {
		return usageByModel{
			"opus":  use(1000, 2000, 30000, 4000), // 0.285
			"haiku": use(5000, 600, 70000, 8000),  // (5000 + 3000 + 7000 + 10000)/1e6 = 0.025
		}
	}
	cases := []struct {
		name     string
		u        usageByModel
		prices   Prices
		cost     *float64
		complete bool
		unpriced []string
	}{
		{name: "both priced", u: two(), prices: tokPrices, cost: f64(0.31), complete: true, unpriced: []string{}},
		{name: "one unpriced", u: two(), prices: Prices{"opus": tokPrices["opus"]},
			cost: f64(0.285), complete: false, unpriced: []string{"haiku"}},
		{name: "unpriced named in order", u: usageByModel{"opus": use(1000, 2000, 30000, 4000), "zeta": use(1, 0, 0, 0), "beta": use(0, 1, 0, 0)},
			prices: tokPrices, cost: f64(0.285), complete: false, unpriced: []string{"beta", "zeta"}},
		{name: "no table", u: two(), prices: nil, cost: nil, complete: false, unpriced: []string{"haiku", "opus"}},
		{name: "zero tokens", u: usageByModel{}, prices: nil, cost: f64(0), complete: true, unpriced: []string{}},
		{name: "an unpriced model with no tokens", u: usageByModel{"opus": use(1000, 2000, 30000, 4000), "zeta": {}},
			prices: tokPrices, cost: f64(0.285), complete: true, unpriced: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := figure(tc.u, tc.prices, 0, false)
			require.Equal(t, tc.cost, f.Cost)
			require.Equal(t, tc.complete, f.CostComplete)
			require.Equal(t, tc.unpriced, f.UnpricedModels)
		})
	}
}

func TestFigureSumsAndPace(t *testing.T) {
	u := usageByModel{"opus": use(1, 300, 3, 4), "haiku": use(10, 100, 30, 40)}
	f := figure(u, nil, 60000, true)
	require.Equal(t, use(11, 400, 33, 44), sums(t, &f))
	require.Equal(t, i64(150000), f.WorkingMSPer1kOutput, "a minute over 400 output")

	f = figure(u, nil, 60000, false)
	require.Nil(t, f.WorkingMSPer1kOutput, "a figure with no time of its own")

	f = figure(usageByModel{"opus": use(100, 0, 0, 0)}, nil, 60000, true)
	require.Nil(t, f.WorkingMSPer1kOutput, "no output tokens: not applicable")

	f = figure(usageByModel{"opus": use(0, 3, 0, 0)}, nil, 1000, true)
	require.Equal(t, i64(333333), f.WorkingMSPer1kOutput, "rounded to whole milliseconds")
}
