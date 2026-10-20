#!/usr/bin/env bash
# The QoS request as it crosses the core, one line per step:
# PCF (N5 app session) -> SMF (N7 policy update, PCC rule) -> UPF (N4 PFCP modification).
#   corelog.sh [lines]
set -euo pipefail

log=$(ls -td "${FREE5GC_DIR:-$HOME/free5gc}"/log/*/ | head -1)free5gc.log
grep -a -E 'CAT="PolAuth"|PCCRule|HandleSMPolicyUpdateNotify|PFCP Session Modification Request|handleSessionModificationRequest' "$log" |
    grep -av 'does not support' |
    sed -E 's/^time="[0-9-]+T([0-9:]{8})[^"]*".* msg="([^"]*)".* NF="([A-Z]+)".*/\1  \3  \2/' |
    tail -n "${1:-6}"
