#!/bin/bash

# The timeline goes first so it is in the verifier output whatever the tests
# find. It only reports, and never fails the run.
python3 /tests/timeline.py /app/.spektacular/debug /root/.bob/logs || true

pytest /tests/test_goal_check.py -v

if [ $? -eq 0 ]; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
