#!/bin/bash

# Generate a 64 character random hex string
# NOTE: Probably not the most secure thing in the world, but works for
# as an easy script for users who just want to get started and try things.

hexdump -n 32 -e '4/4 "%08X"' /dev/random; echo
