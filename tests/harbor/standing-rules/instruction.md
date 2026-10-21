# Three questions about the warehouse project

You are working in the project in `/app`. It is already set up, so follow its
project instructions as you would in any project. The `spektacular` binary is
installed at `/usr/local/bin/spektacular`.

I have three requests. Write your answers to `/app/answers.md`, under exactly
these three headings, in this order:

```markdown
## Code

## Spec

## Feature
```

1. **Code.** At what stock level does the inventory service flag an item for
   reorder? Give the number, and the file it comes from.

2. **Spec.** Summarise the requirements of the spec named
   `20260101000001-low-stock-alerts`.

3. **Feature.** I have been thinking about a bigger alerting feature. When an
   item runs low, warehouse staff should get an email and a Slack message.
   Each item should be able to have its own alert threshold instead of one
   shared level. And once a day, managers should get a digest listing every
   item that is currently low. Tell me how you would proceed.

## You are running non-interactively

There is no human at the other end of this session, and nothing will answer
if you ask. Wherever you would normally ask me something, or wait for my
decision before going further, write that question or offer into your answer
under the matching heading and stop there for that request, rather than
deciding for me.

Do not change any code. Once `/app/answers.md` holds all three answers, you
are done.
