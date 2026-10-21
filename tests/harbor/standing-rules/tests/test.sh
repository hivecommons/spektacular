#!/bin/bash

# The timeline goes first so it is in the verifier output whatever the tests
# find. It only reports, and never fails the run.
python3 /tests/timeline.py /app/.spektacular/debug || true

pytest /tests/test_standing_rules.py -v

if [ $? -eq 0 ]; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
