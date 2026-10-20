#!/usr/bin/env bash
# What the UPF installed in the gtp5g kernel module for the UE:
# packet detection rules (PDRs) and the QoS enforcement rules (QERs) they use.
set -euo pipefail

tool=${GTP5G_TUNNEL:-$HOME/go/bin/gogtp5g-tunnel}
pdrs=$(sudo "$tool" list pdr upfgtp)
echo "PDRs"
jq -r 'sort_by(.ID)[] |
    "  PDR \(.ID)  precedence \(.Precedence)  QER \(.QERID // [] | join(","))  " +
    (.PDI.SDF.FD // null | if . then "\(.Src.IP) \(.SrcPorts // [] | flatten | join(",")) -> \(.Dst.IP) \(.DstPorts // [] | flatten | join(","))" else "default (all traffic)" end)' <<<"$pdrs"
# Upstream free5gc leaves a released QER behind, unused; show only the QERs a PDR points to.
echo "QERs"
sudo "$tool" list qer upfgtp | jq -r --argjson pdrs "$pdrs" '
    ([$pdrs[].QERID // [] | .[]] | unique) as $used |
    sort_by(.ID)[] | select(.ID as $id | $used | index($id)) |
    "  QER \(.ID)  QFI \(.QFI)  MBR DL \(.MBR.DL_Kbps) Kbps  GBR DL \(.GBR.DL_Kbps) Kbps"'
