package plantask

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// taskPlan is a well-formed task plan. The expected values in the tests below
// are written out by hand from this text, never derived from the parser.
const taskPlan = `# Plan: demo

## Overview

- [ ] an overview checkbox that is not a task

## Milestones & Tasks

### Milestone 1: Plans can be read

#### - [x] Task: Add a plan task reader
**Id:** 0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Summary.

- [ ] a stray checkbox before the criteria marker

**Acceptance criteria**:
- [x] reads ids
- [x] reads repos
- [ ] reads everything else

#### - [ ] Task: Add the ` + "`plan export`" + ` command
**Id:** 7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34
**Repo:** spektacular
**Depends on:**
- 0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95 — A title that has drifted from the real one
**Execution:** agent

**Acceptance criteria**:
- [ ] exports

### Milestone 2: People do their part

#### - [ ] Task: Publish the release signing key
**Id:** e4a8c3f1-2b6d-4f90-a7c5-91d0b3e6f428
**Repo:** docs
**Depends on:**
- 7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34 — Add the plan export command
- 0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95 — Add a plan task reader
**Execution:** human — needs access to the production key vault

**Acceptance criteria**:
- [X] key published
- [ ] key rotated

## Testing Approach

- [ ] a testing checkbox that is not a criterion

#### - [ ] Task: A task heading outside the section
**Id:** not-a-task

## Changelog

- [x] a changelog checkbox
`

func TestParse_ReadsEveryTaskField(t *testing.T) {
	p := Parse([]byte(taskPlan))

	require.Equal(t, FormatTasks, p.Format)
	require.Equal(t, []Milestone{
		{Number: 1, Title: "Plans can be read", Items: 2, Open: 1},
		{Number: 2, Title: "People do their part", Items: 1, Open: 1},
	}, p.Milestones)

	require.Len(t, p.Tasks, 3)

	reader := p.Tasks[0]
	require.Equal(t, "0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95", reader.ID)
	require.Equal(t, "Add a plan task reader", reader.Title)
	require.Equal(t, 1, reader.Milestone)
	require.Equal(t, "spektacular", reader.Repo)
	require.Empty(t, reader.DependsOn, "none yields no dependencies")
	require.Equal(t, Execution{Type: "agent"}, reader.Execution)
	require.True(t, reader.Completed)
	require.Equal(t, Criteria{Met: 2, Total: 3}, reader.Criteria,
		"a checkbox before the criteria marker is not a criterion")

	export := p.Tasks[1]
	require.Equal(t, "7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34", export.ID)
	require.Equal(t, "Add the `plan export` command", export.Title)
	require.Equal(t, []string{"0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95"}, export.DependsOn,
		"a drifted dependency title never affects resolution")
	require.False(t, export.Completed)
	require.Equal(t, Criteria{Met: 0, Total: 1}, export.Criteria)

	key := p.Tasks[2]
	require.Equal(t, 2, key.Milestone)
	require.Equal(t, "docs", key.Repo)
	require.Equal(t, []string{
		"7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34",
		"0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95",
	}, key.DependsOn)
	require.Equal(t, Execution{Type: "human", Reason: "needs access to the production key vault"}, key.Execution)
	require.Equal(t, Criteria{Met: 1, Total: 2}, key.Criteria, "an uppercase X counts as met")
}

func TestParse_OnlyReadsTheMilestonesSection(t *testing.T) {
	p := Parse([]byte(taskPlan))
	for _, task := range p.Tasks {
		require.NotEqual(t, "not-a-task", task.ID)
	}
	require.Len(t, p.Tasks, 3)
}

func TestParse_Lookups(t *testing.T) {
	p := Parse([]byte(taskPlan))

	got, ok := p.Task("7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34")
	require.True(t, ok)
	require.Equal(t, "Add the `plan export` command", got.Title)

	_, ok = p.Task("missing")
	require.False(t, ok)

	open := p.OpenTasks()
	require.Len(t, open, 2)
	require.Equal(t, "7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34", open[0].ID)
	require.Equal(t, "e4a8c3f1-2b6d-4f90-a7c5-91d0b3e6f428", open[1].ID)
	require.Equal(t, 2, p.OpenItems())
	require.Empty(t, p.CompletedMilestones())
	require.NoError(t, p.RequireTasks())
}

const legacyPlan = `# Plan: old

## Milestones & Phases

### Milestone 1: First

#### - [x] Phase 1.1: Do a thing
**Repo:** spektacular

**Acceptance criteria**:
- [x] done

#### - [X] Phase 1.2: Do another

### Milestone 2: Second

#### - [x] Phase 2.1: Done
#### - [ ] Phase 2.2: Not done

## Changelog

#### - [ ] Phase 9.9: a heading outside the section
`

func TestParse_LegacyPlan(t *testing.T) {
	p := Parse([]byte(legacyPlan))

	require.Equal(t, FormatLegacy, p.Format)
	require.Empty(t, p.Tasks)
	require.Equal(t, []Milestone{
		{Number: 1, Title: "First", Items: 2, Open: 0},
		{Number: 2, Title: "Second", Items: 2, Open: 1},
	}, p.Milestones)
	require.Equal(t, []int{1}, p.CompletedMilestones())
	require.Equal(t, 1, p.OpenItems())

	err := p.RequireTasks()
	requireCode(t, err, CodeStructureInvalid)
	require.Contains(t, err.Error(), "the plan contains no task ids")
	for _, word := range []string{"legacy", "old", "version", "age"} {
		require.NotContains(t, err.Error(), word)
	}
}

func TestParse_NoWorkAtAll(t *testing.T) {
	p := Parse([]byte("# Plan\n\n## Overview\n\nNothing here.\n"))
	require.Equal(t, FormatNone, p.Format)
	require.Empty(t, p.Milestones)
	requireCode(t, p.RequireTasks(), CodeStructureInvalid)
}

func TestParse_MixedHeadingsAreATaskPlan(t *testing.T) {
	p := Parse([]byte(`## Milestones & Tasks

### Milestone 1: Mixed

#### - [x] Phase 1.1: Old style
#### - [ ] Task: New style
**Id:** a
`))
	require.Equal(t, FormatTasks, p.Format)
	require.Len(t, p.Tasks, 1)
	require.Equal(t, []Milestone{{Number: 1, Title: "Mixed", Items: 2, Open: 1}}, p.Milestones)
}

func TestParse_RepeatedMilestoneNumberContinuesTheMilestone(t *testing.T) {
	p := Parse([]byte(`## Milestones & Phases

### Milestone 1: A
#### - [x] Phase 1.1: a
### Milestone 1: A again
#### - [ ] Phase 1.2: b
`))
	require.Equal(t, []Milestone{{Number: 1, Title: "A", Items: 2, Open: 1}}, p.Milestones)
	require.Empty(t, p.CompletedMilestones())
}

func TestParse_CompletedMilestonesAscending(t *testing.T) {
	p := Parse([]byte(`## Milestones & Tasks

### Milestone 3: C
#### - [x] Task: c
### Milestone 1: A
#### - [x] Task: a
### Milestone 2: B
`))
	require.Equal(t, []int{1, 3}, p.CompletedMilestones(), "a milestone with nothing under it is never complete")
}

func TestParse_LegacyPhasesOutsideAMilestoneStillCountAsOpenWork(t *testing.T) {
	p := Parse([]byte(`## Milestones & Phases

#### - [ ] Phase 1.1: First
#### - [ ] Phase 1.2: Second
#### - [x] Phase 1.3: Done

## Changelog

#### - [ ] Phase 9.9: outside the section
`))
	require.Equal(t, FormatLegacy, p.Format)
	require.Equal(t, 2, p.OpenItems())
	require.Empty(t, p.Milestones)
}

func TestParse_OtherHeadingsInsideAMilestoneKeepIt(t *testing.T) {
	p := Parse([]byte(`## Milestones & Phases

### Milestone 1: One

### Notes

#### - [x] Phase 1.1: still milestone one
`))
	require.Equal(t, []int{1}, p.CompletedMilestones())
}
