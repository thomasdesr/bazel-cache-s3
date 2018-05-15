#!/bin/bash
set -e

bazel run //:gazelle -- update
