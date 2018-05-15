#!/bin/bash
set -e

bazel run //:gazelle -- update
bazel run //:gazelle -- update-repos github.com/aws/aws-sdk-go
bazel run //:gazelle -- update-repos github.com/go-chi/chi
bazel run //:gazelle -- update-repos github.com/golang/groupcache
bazel run //:gazelle -- update-repos github.com/pkg/errors
bazel run //:gazelle -- update-repos gopkg.in/tylerb/graceful.v1

