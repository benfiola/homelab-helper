FROM debian:bookworm-slim AS homelab_helper_builder

ARG TARGETPLATFORM

COPY $TARGETPLATFORM/homelab-helper /usr/bin/

FROM debian:bookworm-slim AS lvm2_builder

ARG LVM2_VERSION=2.03.38

RUN <<EOF
apt -y update
apt -y install curl build-essential pkg-config libaio-dev libdevmapper-dev libudev-dev tar xfslibs-dev

curl -fsSL -o /tmp/archive.tar.gz https://sourceware.org/pub/lvm2/releases/LVM2.${LVM2_VERSION}.tgz
mkdir -p /tmp/source
tar xvzf /tmp/archive.tar.gz -C /tmp/source --strip-components=1
rm -f /tmp/archive.tar.gz

mkdir -p /tmp/build
cd /tmp/build

/tmp/source/configure \
  --prefix=/usr \
  --sysconfdir=/etc \
  --sbindir=/sbin \
  --localstatedir=/var \
  --enable-static_link \
  --disable-dependency-tracking \
  --with-thin=internal

make

mkdir -p /tmp/package
DESTDIR=/tmp/package make install

cd /tmp/package
tar cvzf /archive.tar.gz .

EOF

FROM debian:bookworm-slim

RUN <<EOF
apt -y update
apt -y install curl jq tar vim
EOF

COPY --from=lvm2_builder /archive.tar.gz /tmp/archive.tar.gz
RUN <<EOF
cd /
tar xvzf /tmp/archive.tar.gz
EOF

COPY --from=homelab_helper_builder /usr/bin/homelab-helper /usr/bin/homelab-helper
