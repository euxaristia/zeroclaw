FROM debian:trixie-slim

# No bubblewrap / zero-linux-sandbox: zero's inner sandbox cannot create
# namespaces in an unprivileged container, so every shell command would fail
# once and retry escalated. The container is the isolation boundary.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl git ripgrep procps openssh-client unzip xz-utils \
      python3 jq gcc libc6-dev \
 && rm -rf /var/lib/apt/lists/*

# gh from the latest GitHub release; the Debian repo version lags too far behind.
RUN arch="$(dpkg --print-architecture)" \
 && ver="$(curl -fsSL https://api.github.com/repos/cli/cli/releases/latest | jq -r '.tag_name | ltrimstr("v")')" \
 && curl -fsSL "https://github.com/cli/cli/releases/download/v${ver}/gh_${ver}_linux_${arch}.tar.gz" \
    | tar -xz -C /usr/local/bin --strip-components=2 "gh_${ver}_linux_${arch}/bin/gh" \
 && gh --version

RUN useradd --create-home --uid 1000 --shell /bin/bash zeroclaw

# Latest Go from go.dev; the Debian package stays pinned to whatever shipped
# with trixie.
RUN arch="$(dpkg --print-architecture)" \
 && ver="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1)" \
 && curl -fsSL "https://go.dev/dl/${ver}.linux-${arch}.tar.gz" | tar -xz -C /usr/local \
 && /usr/local/go/bin/go version

# rustup lives outside /home/zeroclaw because the home volume mount would
# shadow anything baked in there. Owned by the agent so `rustup update` and
# `rustup component add` work at runtime.
ENV RUSTUP_HOME=/usr/local/rustup CARGO_HOME=/usr/local/cargo
RUN curl -fsSL https://sh.rustup.rs \
    | sh -s -- -y --no-modify-path --profile minimal --default-toolchain stable \
 && chown -R zeroclaw:zeroclaw "$RUSTUP_HOME" "$CARGO_HOME" \
 && "$CARGO_HOME/bin/cargo" --version

ENV PATH=/usr/local/go/bin:/usr/local/cargo/bin:$PATH

COPY bin/ /tmp/bin/
RUN cp /tmp/bin/* /usr/local/bin/ 2>/dev/null || true \
 && chmod 0755 /usr/local/bin/* 2>/dev/null || true \
 && rm -rf /tmp/bin

COPY bootstrap/ /opt/zeroclaw/bootstrap/

# The agent's soul: identity, values, voice. Lives OUTSIDE the agent's home on
# purpose: the agent owns /home/zeroclaw and could delete-and-recreate any file
# there, while /opt/zeroclaw is root-owned, so this stays operator-curated and
# read-only to the agent. Changing it means editing env/SOUL.md and rebuilding.
COPY SOUL.md /opt/zeroclaw/SOUL.md

# MIT notice for the bundled zero binary; required if this image is ever
# distributed (pushed to a registry, exported), inert for local-only builds.
COPY LICENSE.zero /opt/zeroclaw/LICENSE.zero

USER zeroclaw
WORKDIR /home/zeroclaw

CMD ["sleep", "infinity"]
