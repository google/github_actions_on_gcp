#!/bin/bash

# References:
# https://github.com/actions/actions-runner-controller/blob/master/runner/entrypoint-dind-rootless.sh

# Fail the script if any of the commands don't succeed.
set -e

# Configure rootless Docker for Docker-in-Docker support.
export XDG_RUNTIME_DIR=/home/runner/.docker/run
export PATH=/home/runner/bin:$PATH
export DOCKER_HOST=unix:///home/runner/.docker/run/docker.sock
docker context use rootless
/home/runner/bin/dockerd-rootless.sh &

# TODO: Catch if run fails and fail the script so it bubbles up in Cloud Build.

cd $HOME/actions-runner
./run.sh --jitconfig $ENCODED_JIT_CONFIG &
wait $!
