#!/bin/bash

# Run the implement-workflow verifier
pytest /tests/test_implement_workflow.py -v --junitxml=/logs/verifier/junit.xml

if [ $? -eq 0 ]; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi

# The run's metrics record goes beside the reward, and is written only once
# the reward is settled. It reads the agent's logs, the session log and the
# results above, and never fails the run.
python3 /tests/metrics.py || true
