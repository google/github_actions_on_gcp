#!/usr/bin/env bash

set -euo pipefail

# Build runner image
docker build -t runner:latest ./runner

RUNNER_NAME="local-$(tr -dc A-Za-z0-9 </dev/urandom | head -c 12 || true)"

echo "Generating JIT Config. If no more output, something failed. Try running \
command without storing to variable to see output."

# Generate JIT Config
JIT_CONFIG="$(go run ./cmd/generate-jit \
-app-id #YOUR_APP_ID_GOES_HERE \
-private-key /tmp/gh_key.pem \
-org #YOUR_ORG_NAME_GOES_HERE \
-runner-name "${RUNNER_NAME}" \
-runner-group-id 1 \
)"

echo "${JIT_CONFIG}"

echo "I haven't found a way to make it interruptable once the runner is \
listening. Use docker ps and docker kill in another terminal window."

# Run runner with jitconfig
docker run -it --privileged -e ENCODED_JIT_CONFIG="${JIT_CONFIG}" runner:latest
