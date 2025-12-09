ARG ALPINE_VERSION=3.22.2

FROM alpine:${ALPINE_VERSION} as homelab_helper_builder

ARG TARGETPLATFORM

COPY $TARGETPLATFORM/myprogram /usr/bin/

FROM alpine:${ALPINE_VERSION} AS vault_builder

ARG TARGETARCH
ARG VAULT_VERSION=1.21.1

RUN <<EOF
apk update
apk add curl unzip 
curl -fsSL -o /tmp/archive.zip https://releases.hashicorp.com/vault/${VAULT_VERSION}/vault_${VAULT_VERSION}_linux_${TARGETARCH}.zip
unzip -d /usr/bin /tmp/archive.zip
EOF

FROM alpine:${ALPINE_VERSION}

RUN <<EOF
apk update
apk add curl jq kubectl lvm2 parted 
EOF

COPY --from=homelab_helper_builder /usr/bin/homelab-helper /usr/bin/homelab-helper
COPY --from=vault_builder /usr/bin/vault /usr/bin/vault
