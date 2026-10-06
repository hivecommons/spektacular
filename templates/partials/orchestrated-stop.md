{{! Appended by stepkit.WriteStepResult to every step of an orchestrated workflow. Do not include it from a step template. }}
## Running under an orchestrator

This run was started by an epic orchestrator, which drives several specs at once and is the only one that talks to the user. Do not ask the user anything yourself:

- **Wherever this step says to STOP, to ask the user, or to report to the user**, hand it back instead. End your turn with a final message whose first line is exactly `QUESTION: {{plan_name}}`, followed by the question, the options, and the default you recommend. Then wait: the orchestrator answers with the user's decision, and you carry on from this step.
- Only a genuine open question earns a hand-back: a stop this step defines, or a decision with no reasonable default. Settle everything else yourself and record it as an assumption, so the user can review it at the end.
- **If the run cannot go on at all**, end with a final message whose first line is exactly `FAILED: {{plan_name}}`, followed by the step you reached and the reason.
- Never offer to save knowledge or ask whether to continue between tasks. List any durable discovery in your closing summary instead, and the orchestrator raises it with the user.
