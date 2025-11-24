#!/bin/bash
set -x
echo "Sourcing results:"
declare -f say_hello || echo "say_hello NOT FOUND"
echo "SAMPLE_VAR: $SAMPLE_VAR"
say_hello
