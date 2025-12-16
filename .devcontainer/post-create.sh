#!/bin/sh
set -e

apk update
apk add make

BIN=/usr/local/bin make install-tools
