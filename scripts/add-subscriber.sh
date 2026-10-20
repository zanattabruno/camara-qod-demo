#!/usr/bin/env bash
# Provision the demo UE through the free5gc webconsole REST API
# (the same call the webconsole's "Create subscriber" form makes).
set -euo pipefail

WEB=${WEBCONSOLE:-http://127.0.0.1:5000}
SUPI=imsi-208930000000001       # must match config/ueransim/ue.yaml
KEY=8baf473f2f8fd09487cccbd7097c6862
OPC=8e27b6af0e692e750f32667a3b14605d

for _ in $(seq 20); do curl -s -o /dev/null "$WEB" && break; sleep 0.5; done   # webconsole up?
token=$(curl -sf "$WEB/api/login" -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"free5gc"}' | jq -r .access_token)

if [ "${1:-}" = "--delete" ]; then
    curl -sf -o /dev/null -w "DELETE /api/subscriber/$SUPI: HTTP %{http_code}\n" \
        -X DELETE "$WEB/api/subscriber/$SUPI/20893" -H "Token: $token"
    exit
fi
if curl -sf "$WEB/api/subscriber" -H "Token: $token" | jq -e --arg supi "$SUPI" 'any(.[]; .ueId == $supi)' >/dev/null; then
    echo "$SUPI is already provisioned"
    exit
fi

# Start from the webconsole's sample subscriber (slice 1/010203, DNN internet)
# and set this UE's identity and keys.
curl -sf "$WEB/api/sample" -H "Token: $token" |
    jq --arg supi "$SUPI" --arg key "$KEY" --arg opc "$OPC" '
        .ueId = $supi
        | .AuthenticationSubscription.permanentKey.permanentKeyValue = $key
        | .AuthenticationSubscription.opc.opcValue = $opc
        | .AuthenticationSubscription.milenage.op.opValue = ""' |
    curl -sf -o /dev/null -w "POST /api/subscriber/$SUPI: HTTP %{http_code}\n" \
        -X POST "$WEB/api/subscriber/$SUPI/20893" -H "Token: $token" \
        -H 'Content-Type: application/json' -d @-
