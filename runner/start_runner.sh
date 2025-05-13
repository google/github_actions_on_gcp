#!/bin/bash
set -e

echo "Attempting to start Docker daemon..."

echo "Current disk space before starting dockerd:"
df -h

# Determine the GID of the 'docker' group. This group is created in the Dockerfile.
DOCKER_GROUP_ID=$(getent group docker | cut -d: -f3)

if [ -z "$DOCKER_GROUP_ID" ]; then
    echo "Error: The 'docker' group GID was not found."
    exit 1
else
    echo "The 'docker' group GID is: $DOCKER_GROUP_ID"
    DOCKER_SOCKET_GROUP="$DOCKER_GROUP_ID"
fi

# Start the Docker daemon in the background using sudo.
sudo sh -c "dockerd \
    --host=unix:///var/run/docker.sock \
    --host=tcp://0.0.0.0:2375 \
    --group=\"$DOCKER_SOCKET_GROUP\" \
    --storage-driver=vfs \
    > /var/log/dockerd.log 2>&1" &

# Wait for the Docker socket to be available and the daemon to be responsive
DOCKER_SOCKET="/var/run/docker.sock"
TIMEOUT_SECONDS=60
WAIT_INTERVAL_SECONDS=1
ELAPSED_SECONDS=0

echo "Waiting for Docker daemon to become available at ${DOCKER_SOCKET}..."
while true; do
    if [ ${ELAPSED_SECONDS} -ge ${TIMEOUT_SECONDS} ]; then
        echo "Timeout: Docker daemon did not become available after ${TIMEOUT_SECONDS} seconds."
        echo "Please check Docker daemon logs for errors: sudo cat /var/log/dockerd.log"
        sudo cat /var/log/dockerd.log
        echo "Current status of ${DOCKER_SOCKET}:"
        sudo ls -l "${DOCKER_SOCKET}" || echo "Socket ${DOCKER_SOCKET} not found."
        echo "Unable to configure docker daemon, exiting."
        exit 1
    fi

    # Check if socket file exists and then if 'sudo docker info' works
    if [ -S "${DOCKER_SOCKET}" ] && sudo -n docker info >/dev/null 2>&1; then
        echo # Newline for cleaner output
        echo "Docker daemon socket detected at ${DOCKER_SOCKET} and is responsive to 'sudo docker info'."
        # Allow system to stabilize socket permissions fully
        sleep 2
        break
    fi

    # Progress indicator
    echo -n "."

    sleep ${WAIT_INTERVAL_SECONDS}
    ELAPSED_SECONDS=$((ELAPSED_SECONDS + WAIT_INTERVAL_SECONDS))
done

# Final check: can the current user ('runner') access Docker without sudo?
if docker info >/dev/null 2>&1; then
    echo "SUCCESS: Docker daemon is responsive to the 'runner' user."
else
    echo "ERROR: 'docker info' as 'runner' user (UID $(id -u)) failed."
    exit 1
fi

# This ensures Docker CLI commands (i.e. login) run by actions
# will use a writable location for their configuration.
export HOME="/home/runner"
export DOCKER_CONFIG="${HOME}/.docker-runner-default"

# Create the directory if it doesn't exist.
# Since this script runs as the 'runner' user, 'runner' will own this directory.
mkdir -p "${DOCKER_CONFIG}"
echo "Default DOCKER_CONFIG for this runner session set to: ${DOCKER_CONFIG}"

# Finally register a github runner using the jit config env variable.
/actions-runner/run.sh --jitconfig $ENCODED_JIT_CONFIG &
wait $!
