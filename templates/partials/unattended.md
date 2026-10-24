{{! Appended by stepkit.WriteStepResult to every step of an unattended spec workflow. Do not include it from a step template. }}
## Running unattended

The user handed this spec off: they asked you to write it from what has already been discussed and said they will review it afterwards. Drive this step to the end yourself. Where this section and the step above disagree, this section wins.

- **Wherever the step says to present a draft and ask the user to confirm or refine it, do not ask.** Draft the section from this conversation, `.spektacular/working-context.md`, `.spektacular/work/{{spec_name}}/interview.md` and any material the spec was seeded from, save it to its working file, and advance. Every other rule the step sets still holds: altitude, format, the tests each section applies.
- **At the interview step, do not interview.** Write `interview.md` from what the conversation and the seeded material already settle, and say in it what neither settles.
- **Record every judgement call.** Each time you settle something the user did not, append one entry to `.spektacular/work/{{spec_name}}/assumptions.md` with your own `Write` tool (create the file on first use):

  ```markdown
  ### <short title> (<section>)
  - **Assumed**: what you wrote
  - **Because**: what in the conversation or the material it rests on
  ```

  A constraint or non-goal you cannot trace to something the user said is never written as settled. Put it in Technical Approach as direction with an assumption entry, or carry it as an open question.
- **Carry questions instead of asking them.** When something is genuinely undecided and cannot be inferred, append it to `.spektacular/work/{{spec_name}}/open_questions.md`, one bullet each naming the section it affects, and carry on with the most conservative reading. Stop to ask the user only when no credible draft of a section can be written without the answer.
- **Accept no offer on the user's behalf.** An offer this workflow makes, such as capturing a design document, splitting the spec into an epic or saving knowledge, is not accepted for them, and nothing is written for it. Add each one to `open_questions.md` instead, so the review raises it.
- **A step that says STOP** because a working file is missing or a command failed still stops: put it right yourself if you can, and otherwise tell the user what went wrong.
