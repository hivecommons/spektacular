#!/bin/bash

# Run the plan-workflow verifier
pytest /tests/test_plan_workflow.py -v

if [ $? -eq 0 ]; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
