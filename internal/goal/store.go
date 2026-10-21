package goal

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/store"
)

// FileName is the outcome record's file name in a plan's folder of the plan
// store.
const FileName = "goals.json"

// FilePath is the store-relative path of a plan's outcome record: goals.json
// beside the plan's documents. It is a layout helper for the CLI only and is
// never rendered into an agent's instructions.
func FilePath(planDir, plan string) string {
	return artifact.FeatureDir(planDir, plan) + "/" + FileName
}

// Load reads a plan's outcome record. A plan never checked has an empty
// record. Each item's attempt count and state are derived on load, never
// trusted from disk.
func Load(st store.Reader, planDir, plan string) (Outcome, error) {
	o := Outcome{Plan: plan, Items: []ItemOutcome{}}
	raw, err := st.Read(FilePath(planDir, plan))
	if errors.Is(err, store.ErrNotFound) {
		return o, nil
	}
	if err != nil {
		return o, err
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return o, fmt.Errorf("reading %s's goal-check record: %w", plan, err)
	}
	o.Plan = plan
	if o.Items == nil {
		o.Items = []ItemOutcome{}
	}
	for i := range o.Items {
		o.Items[i] = derive(o.Items[i])
	}
	return o, nil
}

// Save writes a plan's outcome record.
func Save(st store.Writer, planDir string, o Outcome) error {
	for i := range o.Items {
		o.Items[i] = derive(o.Items[i])
	}
	raw, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	return st.Write(FilePath(planDir, o.Plan), append(raw, '\n'))
}

// AppendCheck adds one check to its item's outcome, stamping its time, and
// returns the item as recorded.
func AppendCheck(st store.Store, planDir, plan string, c Check, now time.Time) (ItemOutcome, error) {
	o, err := Load(st, planDir, plan)
	if err != nil {
		return ItemOutcome{}, err
	}
	if c.At == "" {
		c.At = now.UTC().Format(time.RFC3339)
	}
	it := o.Item(c.Goal.Item)
	it.Title = c.Goal.Title
	it.Checks = append(it.Checks, c)
	o.put(it)
	if err := Save(st, planDir, o); err != nil {
		return ItemOutcome{}, err
	}
	return o.Item(c.Goal.Item), nil
}

// AppendDecision records the user's answer to an item's stopped check,
// stamping its time, and returns the item as recorded and the implement step
// the answer leads to. It is refused when no decision is due or the decision
// is unknown.
func AppendDecision(st store.Store, planDir, plan, item string, d Decision, now time.Time) (ItemOutcome, string, error) {
	o, err := Load(st, planDir, plan)
	if err != nil {
		return ItemOutcome{}, "", err
	}
	if d.At == "" {
		d.At = now.UTC().Format(time.RFC3339)
	}
	it, next, err := ApplyDecision(o.Item(item), d)
	if err != nil {
		return it, "", err
	}
	o.put(it)
	if err := Save(st, planDir, o); err != nil {
		return ItemOutcome{}, "", err
	}
	return o.Item(item), next, nil
}
