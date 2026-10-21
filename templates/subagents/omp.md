---
name: spek-goal-reviewer
description: Independent, read-only reviewer for the Spektacular implement workflow's check_goal step. Judges each acceptance criterion of one plan task as met, not_met or impossible, with evidence, from the files and the staged diff. Launch it with only the task's goal, the verification results and pointers to the work.
tools: read, grep, glob
model: "{{{model}}}"
---

{{> partials/goal-reviewer-brief}}
