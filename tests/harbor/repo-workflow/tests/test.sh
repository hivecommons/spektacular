#!/bin/bash

# Run the test suite
rm -f /logs/verifier/reward.json
pytest /tests/test_repo_workflow.py -v --junitxml=/logs/verifier/junit.xml

if [ $? -eq 0 ]; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi

# Score the run out of 1 by the fraction of checks that passed, with a score
# per check group, in reward.json, which harbor reads in place of reward.txt.
# write_reward.py is copied in by the Makefile; without it, or if it fails,
# reward.txt above still holds the pass/fail reward.
if [ -f /tests/write_reward.py ]; then
  uv run --no-project --quiet python /tests/write_reward.py \
    /logs/verifier/junit.xml /logs/verifier/reward.json \
    || rm -f /logs/verifier/reward.json
fi
