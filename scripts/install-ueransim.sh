#!/usr/bin/env bash
# Install UERANSIM, the open-source gNB + UE simulator.
set -euo pipefail

TAG=${UERANSIM_TAG:-v3.3.0}
DIR=${UERANSIM_DIR:-$HOME/UERANSIM}

sudo apt-get install -y make gcc g++ cmake libsctp-dev lsb-release
[ -d "$DIR" ] || git clone -b "$TAG" https://github.com/aligungr/UERANSIM "$DIR"
make -C "$DIR"
echo "UERANSIM $TAG ready in $DIR/build"
