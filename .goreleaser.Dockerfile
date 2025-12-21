FROM debian:bookworm-slim AS homelab_helper_builder
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/empty /usr/bin/


FROM ghcr.io/benfiola/homelab-lvm:0.1.1 AS lvm2_builder


FROM debian:bookworm-slim
RUN <<EOF
apt -y update
apt -y install curl jq tar thin-provisioning-tools vim
EOF

COPY --from=lvm2_builder /archive.tar.gz /tmp/archive.tar.gz
RUN <<EOF
cd /
tar xvzf /tmp/archive.tar.gz
ln -fs /sbin/lvm /usr/sbin/lvm.static
EOF

COPY --from=homelab_helper_builder /usr/bin/empty /usr/bin/empty
