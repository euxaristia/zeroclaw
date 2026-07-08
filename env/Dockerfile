FROM debian:trixie-slim

# No bubblewrap / zero-linux-sandbox: zero's inner sandbox cannot create
# namespaces in an unprivileged container, so every shell command would fail
# once and retry escalated. The container is the isolation boundary.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl git ripgrep procps openssh-client unzip xz-utils \
 && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --uid 1000 --shell /bin/bash zeroclaw

COPY bin/zero /usr/local/bin/zero
RUN chmod 0755 /usr/local/bin/zero

COPY bootstrap/ /opt/zeroclaw/bootstrap/

USER zeroclaw
WORKDIR /home/zeroclaw

CMD ["sleep", "infinity"]
