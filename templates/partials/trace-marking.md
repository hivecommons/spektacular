{{! Appended by stepkit.WriteStepResult to every step instruction, straight after the step's own text, and included by every workflow skill after its version check. Do not include it from a step template. }}
**Mark spans.** {{> partials/trace-helpers}}
{{^orchestrated}}
Before you ask the user a question, run `{{command}} trace start <question> --type user-wait{{#trace_parent}} --parent {{trace_parent}}{{/trace_parent}}`, and once they answer run `{{command}} trace stop <id>`, chained with your next command.
{{/orchestrated}}
