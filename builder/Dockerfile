FROM ubuntu

# References:
# https://github.com/docker-library/docker/blob/master/27/dind-rootless/Dockerfile
# https://github.com/docker-library/docker/blob/master/27/dind/Dockerfile
# https://github.com/actions/runner/blob/main/images/Dockerfile
# https://github.com/actions/actions-runner-controller/blob/master/runner/actions-runner-dind-rootless.ubuntu-20.04.dockerfile

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -y && apt-get upgrade -y && useradd -m runner

RUN DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    curl jq build-essential libssl-dev libffi-dev python3 python3-venv python3-dev python3-pip libicu-dev gnupg uidmap \
    iproute2 iptables sudo unzip zip fuse-overlayfs

# Set-up subuid and subgid so that "--userns-remap=default" works
RUN set -eux
addgroup --system dockremap
adduser --system --ingroup dockremap dockremap
echo 'dockremap:165536:65536' >>/etc/subuid
echo 'dockremap:165536:65536' >>/etc/subgid

# Make the rootless runner directory executable
RUN mkdir /run/user/$RUNNER_UID &&
    chown runner:runner /run/user/$RUNNER_UID &&
    chmod a+x /run/user/$RUNNER_UID

# Dynamically find and download the latest runner version.
RUN cd /home/runner && mkdir actions-runner && cd actions-runner &&
    curl -o actions-runner-linux-x64.tar.gz \
        -L $(curl -s https://api.github.com/repos/actions/runner/releases/latest |
            grep browser_download_url |
            grep actions-runner-linux-x64 |
            cut -d'"' -f4) &&
    tar xzf ./actions-runner-linux-x64.tar.gz

RUN chown -R runner ~runner && /home/runner/actions-runner/bin/installdependencies.sh

COPY start_runner.sh start_runner.sh
RUN chmod +x start_runner.sh
USER runner

ENV SKIP_IPTABLES=1
# Something fails without FORCE_ROOTLESS_INSTALL but it doesn't seem to break any functionality.
ENV FORCE_ROOTLESS_INSTALL=1
RUN curl -fsSL https://get.docker.com/rootless | sh && /home/runner/bin/docker -v

# TODO: This might not be necessary.
# Create folder structure here to avoid permission issues when mounting the daemon.json file from a configmap.
RUN mkdir -p /home/runner/.config/docker

ENTRYPOINT ["./start_runner.sh"]
