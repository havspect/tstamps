# Dev sandbox for this repo: opencode and agy (Antigravity CLI) on the gateway's
# own Ubuntu base, which already carries the non-root `ubuntu` user (uid 1000).
# Build: docker build -t tstamps-agents:latest openshell (or `just sandbox-image`).
FROM nvcr.io/nvidia/base/ubuntu:24.04

ARG AGY_VERSION=1.4.12
ARG OPENCODE_VERSION=2.2.5
ARG GO_VERSION=1.26.0

USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git jq tar \
 && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
 && apt-get install -y --no-install-recommends nodejs \
 && npm install -g pnpm@12 "@opencode/cli@${OPENCODE_VERSION}" \
 && curl -fsSL -o /tmp/agy.tgz \
      "https://github.com/google-antigravity/antigravity-cli/releases/download/${AGY_VERSION}/agy_cli_linux_x64.tar.gz" \
 && tar -xzf /tmp/agy.tgz -C /usr/local/bin \
 && ln -sf /usr/local/bin/antigravity /usr/local/bin/agy \
 # x86_64 only: the tarball name above is arch-specific.
 && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -xz -C /usr/local \
 && mkdir -p /workspace \
 && chown ubuntu:ubuntu /workspace \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/* /tmp/agy.tgz

USER ubuntu
ENV PATH="/usr/local/go/bin:${PATH}"
WORKDIR /workspace
