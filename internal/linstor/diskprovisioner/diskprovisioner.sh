#!/bin/sh
set -ex

if [ "${PARTITION_LABEL}" = "" ]; then
  1>&2 echo "error: PARTITION_LABEL unset"
  exit 1
fi
if [ "${POOL}" = "" ]; then
  1>&2 echo "error: POOL unset"
  exit 1
fi
if [ "${VOLUME_GROUP}" = "" ]; then
  1>&2 echo "error: VOLUME_GROUP unset"
  exit 1
fi

symlink="/dev/disk/by-partlabel/${PARTITION_LABEL}"
partition="$(readlink -f "${symlink}")"
if [ "${partition}" = "${symlink}" ]; then
  1>&2 echo "error: could not find target disk"
  exit 1
fi

if ! pvdisplay "${partition}" > /dev/null 2>&1; then
  pvcreate "${partition}"
fi

pvresize "${partition}"

if ! vgdisplay "${VOLUME_GROUP}" > /dev/null 2>&1; then
  vgcreate "${VOLUME_GROUP}" "${partition}"
fi

size="100%FREE"
if ! lvdisplay "${VOLUME_GROUP}/${POOL}" > /dev/null 2>&1; then
  lvcreate -l "${size}" -T "${VOLUME_GROUP}/${POOL}" -Zn --chunksize 512K
fi

lvextend -l "${size}" "${VOLUME_GROUP}/${POOL}" || true
