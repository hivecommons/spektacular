package autocommit

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// milestoneHeading matches a "### Milestone N:" line, which opens a group of
// phases in a plan's Milestones & Phases section.
var milestoneHeading = regexp.MustCompile(`^###\s+Milestone\s+(\d+)\s*:`)

// phaseCheckbox matches a "#### - [ ] Phase N.M:" line and captures the box's
// contents, so a ticked phase can be told from an open one. The tick is
// matched case-insensitively because an uppercase X is just as common by hand.
var phaseCheckbox = regexp.MustCompile(`^####\s+-\s+\[([ xX])\]\s+Phase\b`)

// CompletedMilestones reports which milestones in a plan have every one of
// their phases ticked, ascending. A milestone with no phases at all is never
// complete: there is nothing in it to have finished.
//
// Only the "## Milestones & Phases" section is scanned, so a checkbox
// elsewhere in the plan — an acceptance criterion, a changelog entry — can
// never be mistaken for a phase.
func CompletedMilestones(planMarkdown string) []int {
	type counts struct{ phases, open int }
	byMilestone := map[int]*counts{}

	inSection := false
	current := 0
	for _, line := range strings.Split(planMarkdown, "\n") {
		if strings.HasPrefix(line, "## ") {
			// The section ends at the next second-level heading.
			inSection = strings.HasPrefix(line, "## Milestones & Phases")
			current = 0
			continue
		}
		if !inSection {
			continue
		}
		if m := milestoneHeading.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				current = 0
				continue
			}
			current = n
			if _, ok := byMilestone[n]; !ok {
				byMilestone[n] = &counts{}
			}
			continue
		}
		if current == 0 {
			continue
		}
		if box := phaseCheckbox.FindStringSubmatch(line); box != nil {
			c := byMilestone[current]
			c.phases++
			if box[1] == " " {
				c.open++
			}
		}
	}

	var done []int
	for n, c := range byMilestone {
		if c.phases > 0 && c.open == 0 {
			done = append(done, n)
		}
	}
	sort.Ints(done)
	return done
}

// DueMilestones returns the completed milestones that have not been committed
// yet, ascending — the ones a milestone commit at this point must name.
func DueMilestones(completed, committed []int) []int {
	already := make(map[int]bool, len(committed))
	for _, n := range committed {
		already[n] = true
	}
	var due []int
	for _, n := range completed {
		if !already[n] {
			due = append(due, n)
		}
	}
	return due
}
