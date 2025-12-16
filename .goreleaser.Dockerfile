FROM alpine:3.22.2 as homelab_helper_builder

ARG TARGETPLATFORM

COPY $TARGETPLATFORM/homelab-helper /usr/bin/

FROM alpine:3.22.2

RUN <<EOF
apt -y update
apt -y install curl jq lvm2 parted vim
EOF

COPY --from=homelab_helper_builder /usr/bin/homelab-helper /usr/bin/homelab-helper
