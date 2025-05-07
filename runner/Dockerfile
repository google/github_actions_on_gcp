FROM gcr.io/cloud-builders/npm@sha256:e7ae25cd9213a0bd0cf401b3420c0d66cb8565db6230208d38a4e0b2b3c62168

ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /actions-runner

# Install misc deps.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
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

RUN curl -f -L -o runner.tar.gz https://github.com/actions/runner/releases/download/v2.323.0/actions-runner-linux-x64-2.323.0.tar.gz \
    && tar xzf ./runner.tar.gz \
    && rm runner.tar.gz \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /actions-runner/externals/node20_alpine/ \
    && rm -f /actions-runner/externals/node20/bin/node \
    && ln -s $(which node) /actions-runner/externals/node20/bin/node \
    && find /actions-runner/externals/node20/include/node/openssl/archs/. ! -name 'linux-x86_64' ! -name '.' -type d -exec rm -rf {} +

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
  && rm -rf /var/lib/apt/lists/* \
  && python3 -m pip install pipx \
  && python3 -m pipx ensurepath \
  && python3 --version

# Install docker.
WORKDIR /install/docker/
RUN apt-get -y update \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo \
        "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian \
        $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
        tee /etc/apt/sources.list.d/docker.list > /dev/null \
    && apt-get update -y \
    && apt-get -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/* \
    && docker --version

# Install gh CLI.
ARG GH_VERSION=2.70.0
ARG GH_PACKAGE=gh_${GH_VERSION}_linux_amd64.tar.gz
ARG GH_INSTALL_DIR=/opt/gh-cli-${GH_VERSION}
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
