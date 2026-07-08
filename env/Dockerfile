FROM debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl git ripgrep procps openssh-client unzip xz-utils \
      bubblewrap \
 && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --uid 1000 --shell /bin/bash zeroclaw

COPY bin/zero /usr/local/bin/zero
COPY bin/zero-linux-sandbox /usr/local/bin/zero-linux-sandbox
RUN chmod 0755 /usr/local/bin/zero /usr/local/bin/zero-linux-sandbox

COPY bootstrap/ /opt/zeroclaw/bootstrap/

USER zeroclaw
WORKDIR /home/zeroclaw

CMD ["sleep", "infinity"]
