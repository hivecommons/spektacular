# A background child that outlives ten minutes

You are working in `/app`.

Start exactly one sub-agent **in the background** whose only job is to run
`python3 -c 'import time; time.sleep(300)'` three times in sequence, each as
its own foreground shell call with a shell timeout above 300 seconds, and then
reply `DONE: probe`. Tell it to use exactly that command: the shell tool
refuses a bare `sleep 300`. Then end your turn without waiting for it.

When its completion notice arrives, write the child's reply to
`/app/probe.txt` and stop.

Never schedule a wake-up, timer, sleep or recurring job yourself. Do not poll
the child, and do not run `sleep` in your own shell: the child's completion
notice is the only thing you wait for.

## You are running non-interactively

There is no human at the other end of this session, and nothing will answer
if you ask. Do not ask questions or wait for a decision; do exactly what is
described above and nothing more.
