FROM ubuntu@sha256:dc17125eaac86538c57da886e494a34489122fb6a3ebb6411153d742594c2ddc

ENV DEBIAN_FRONTEND=noninteractive

ARG ACTION_RUNNER_VERSION=2.323.0
ARG GH_VERSION=2.70.0
ARG GH_PACKAGE=gh_${GH_VERSION}_linux_amd64.tar.gz
ARG GH_INSTALL_DIR=/opt/gh-cli-${GH_VERSION}

WORKDIR /actions-runner

# Install misc deps.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      adduser \
      ca-certificates \
      coreutils \
      curl \
      git \
      git-lfs \
      gzip \
      jq \
      lsb-release \
      shellcheck \
      sudo \
      tar \
      unzip \
      yamllint \
      zip \
      zstd \
    && apt-get clean \
    && rm -r /var/lib/apt/lists/*

RUN curl -f -L -o runner.tar.gz https://github.com/actions/runner/releases/download/v${ACTION_RUNNER_VERSION}/actions-runner-linux-x64-${ACTION_RUNNER_VERSION}.tar.gz \
    && tar xzf ./runner.tar.gz \
    && rm runner.tar.gz \
    && rm -rf /var/lib/apt/lists/*

RUN /actions-runner/bin/installdependencies.sh \
    && adduser --disabled-password --gecos "" --uid 1001 runner \
    && groupadd docker --gid 997 \
    && usermod -aG sudo runner \
    && usermod -aG docker runner \
    # Configure sudoers securely
    && echo "%sudo ALL=(ALL:ALL) NOPASSWD:ALL" > /etc/sudoers.d/runner_sudo_access \
    && echo "Defaults env_keep += \"DEBIAN_FRONTEND\"" >> /etc/sudoers.d/runner_sudo_access \
    # Define secure_path for sudo
    && echo "Defaults    secure_path = /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" >> /etc/sudoers.d/secure_path_default \
    && chmod 0440 /etc/sudoers.d/runner_sudo_access \
    && chmod 0440 /etc/sudoers.d/secure_path_default \
    && rm -rf /var/lib/apt/lists/*

# Install Python.
RUN apt-get update -y && \
  apt-get -y install --no-install-recommends \
  python3 \
  python3-dev \
  python3-pip \
  python3-venv \
  pipx \
  && rm -rf /var/lib/apt/lists/* \
  && python3 -m pipx ensurepath \
  && python3 --version

# Install docker.
WORKDIR /install/docker/
RUN apt-get -y update \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo \
        "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
        $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
        tee /etc/apt/sources.list.d/docker.list > /dev/null \
    && apt-get update -y \
    && apt-get -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/* \
    && docker --version

# Install gh CLI.
WORKDIR /install/github-cli/
RUN apt-get update && \
    apt-get install -y --no-install-recommends wget tar ca-certificates && \
    wget -q https://github.com/cli/cli/releases/download/v${GH_VERSION}/${GH_PACKAGE} -O /tmp/${GH_PACKAGE} && \
    tar -xzf /tmp/${GH_PACKAGE} -C /opt && \
    mv /opt/gh_${GH_VERSION}_linux_amd64/bin/gh /usr/local/bin/gh && \
    chmod +x /usr/local/bin/gh && \
    gh --version && \
    rm -rf /tmp/${GH_PACKAGE} /opt/gh_${GH_VERSION}_linux_amd64 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY start_runner.sh /actions-runner/start_runner.sh
RUN chmod +x /actions-runner/start_runner.sh

WORKDIR /home/runner
USER runner

ENTRYPOINT ["/actions-runner/start_runner.sh"]
