#!/usr/bin/env bash
# Install the public free5gc release used by the demo.
# Needs: Go (the version in free5gc's go.mod), Node.js + yarn (webconsole), MongoDB, sudo.
# See "Prerequisites" in the README.
set -euo pipefail

TAG=${FREE5GC_TAG:-v4.2.3}
DIR=${FREE5GC_DIR:-$HOME/free5gc}
GTP5G_TAG=${GTP5G_TAG:-v0.9.16}
GO_MIN=1.26.2   # what free5gc v4.2.3's go.mod files ask for

# 0. Build tools and the demo scripts' helpers; fail early on what apt cannot install.
sudo apt-get install -y build-essential "linux-headers-$(uname -r)" git curl jq openssl
missing=()
command -v go >/dev/null || missing+=("Go >= $GO_MIN")
if command -v go >/dev/null; then
    have=$(go env GOVERSION | sed 's/^go//')
    [ "$(printf '%s\n' "$GO_MIN" "$have" | sort -V | head -1)" = "$GO_MIN" ] || missing+=("Go >= $GO_MIN (found $have)")
fi
command -v node >/dev/null || missing+=("Node.js")
command -v yarn >/dev/null || missing+=("yarn")
command -v mongod >/dev/null || missing+=("MongoDB")
if [ ${#missing[@]} -gt 0 ]; then
    printf 'missing: %s\n' "${missing[@]}" >&2
    exit 1
fi

# 1. gtp5g: the kernel module the UPF uses for the GTP-U data plane.
if ! lsmod | grep -q '^gtp5g'; then
    git clone -b "$GTP5G_TAG" https://github.com/free5gc/gtp5g.git "$HOME/gtp5g"
    make -C "$HOME/gtp5g"
    sudo make -C "$HOME/gtp5g" install
fi

# 2. free5gc: all network functions and the webconsole, built from source.
if [ ! -d "$DIR" ]; then
    git clone --recursive -b "$TAG" https://github.com/free5gc/free5gc.git "$DIR"
fi
make -C "$DIR" all

# 3. Lab setting: no OAuth2 on the SBI, so the CAMARA gateway can call the PCF
#    without an NRF-issued token. Production cores keep this on.
sed -i 's/^\(    oauth:\) true/\1 false/' "$DIR/config/nrfcfg.yaml"

# 4. Tool to list the rules the UPF installed in gtp5g (PDRs, QERs).
go install github.com/free5gc/go-gtp5gnl/cmd/gogtp5g-tunnel@v1.6.2

echo "free5gc $TAG ready in $DIR"
