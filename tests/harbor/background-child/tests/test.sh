#!/bin/bash

# -s lets the verifier print its PROBE OUTCOME line into the verifier output
# whichever way the probe went.
pytest /tests/test_background_child.py -v -s --junitxml=/logs/verifier/junit.xml

if [ $? -eq 0 ]; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi

# The run's metrics record goes beside the reward, and is written only once
# the reward is settled. It reads the agent's logs and the results above, and
# never fails the run.
python3 /tests/metrics.py || true
