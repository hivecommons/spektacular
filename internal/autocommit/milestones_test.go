package autocommit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompletedMilestones(t *testing.T) {
	cases := []struct {
		name string
		plan string
		want []int
	}{
		{
			name: "a milestone with every phase ticked is complete",
			plan: `# Plan: billing

## Milestones & Phases

### Milestone 1: first

#### - [x] Phase 1.1: a

#### - [x] Phase 1.2: b
`,
			want: []int{1},
		},
		{
			name: "a partially ticked milestone is not complete",
			plan: `## Milestones & Phases

### Milestone 1: first

#### - [x] Phase 1.1: a

#### - [ ] Phase 1.2: b
`,
			want: nil,
		},
		{
			name: "a milestone with no phases under it is never complete",
			plan: `## Milestones & Phases

### Milestone 1: first

Some prose, but not a single phase.

### Milestone 2: second

#### - [x] Phase 2.1: a
`,
			want: []int{2},
		},
		{
			name: "an uppercase tick counts as ticked",
			plan: `## Milestones & Phases

### Milestone 1: first

#### - [X] Phase 1.1: a
`,
			want: []int{1},
		},
		{
			name: "a phase checkbox after the section has ended cannot reopen a milestone",
			plan: `## Milestones & Phases

### Milestone 1: first

#### - [x] Phase 1.1: a

## Out of Scope

#### - [ ] Phase 9.9: never built
`,
			want: []int{1},
		},
		{
			name: "a complete milestone group outside the section is not reported",
			plan: `## Overview

### Milestone 9: a worked example, not a real milestone

#### - [x] Phase 9.1: illustrative only

## Milestones & Phases

### Milestone 1: first

#### - [x] Phase 1.1: a
`,
			want: []int{1},
		},
		{
			name: "several milestones completing at once come back ascending",
			plan: `## Milestones & Phases

### Milestone 3: third

#### - [x] Phase 3.1: a

### Milestone 1: first

#### - [x] Phase 1.1: a

### Milestone 2: second

#### - [X] Phase 2.1: a
`,
			want: []int{1, 2, 3},
		},
		{
			name: "an empty Milestones section completes nothing",
			plan: `## Milestones & Phases

## Out of Scope

nothing here either
`,
			want: nil,
		},
		{
			name: "a plan with no Milestones section at all completes nothing",
			plan: `# Plan: billing

## Overview

fixture

#### - [x] Phase 1.1: a
`,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, CompletedMilestones(tc.plan))
		})
	}
}

func TestDueMilestones(t *testing.T) {
	cases := []struct {
		name      string
		completed []int
		committed []int
		want      []int
	}{
		{"nothing committed yet leaves every completed milestone due", []int{1, 2}, nil, []int{1, 2}},
		{"milestones already committed are excluded", []int{1, 2, 3}, []int{1}, []int{2, 3}},
		{"nothing is due when every completed milestone is committed", []int{1, 2}, []int{1, 2}, nil},
		{"nothing completed means nothing due", nil, []int{1}, nil},
		{"a committed milestone that is not complete changes nothing", []int{2}, []int{1}, []int{2}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, DueMilestones(tc.completed, tc.committed))
		})
	}
}
