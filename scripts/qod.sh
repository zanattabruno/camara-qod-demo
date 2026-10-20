#!/usr/bin/env bash
# Small CAMARA QoD client: prints the request it sends and the answer.
#   qod create <file.json>   POST /sessions (remembers the sessionId)
#   qod get                  GET  /sessions/{id}
#   qod extend <seconds>     POST /sessions/{id}/extend
#   qod delete               DELETE /sessions/{id}
set -euo pipefail

API=${QOD_API:-http://127.0.0.1:9091/quality-on-demand/v1}
LAST=${XDG_RUNTIME_DIR:-/tmp}/qod-session-id

call() {  # method path [body-file]
    echo "> $1 /${API#http://*/}$2"
    local out code
    out=$(mktemp)
    code=$(curl -s -o "$out" -w '%{http_code}' -X "$1" "$API$2" \
        -H 'Content-Type: application/json' -H "x-correlator: $(cat /proc/sys/kernel/random/uuid)" \
        ${3:+--data @"$3"})
    echo "< HTTP $code"
    # one line per top-level field, so a whole session fits on screen
    [ -s "$out" ] && jq -r 'if type == "object" then "{", (to_entries | map("  \(.key | tojson): \(.value | tojson)") | join(",\n")), "}" else . end' "$out"
    if [ "$1 $2 $code" = "POST /sessions 201" ]; then jq -r .sessionId "$out" > "$LAST"; fi
    rm -f "$out"
}

id() { cat "$LAST"; }

case ${1:-} in
    create) call POST /sessions "${2:?request file}" ;;
    get)    call GET "/sessions/$(id)" ;;
    extend) body=$(mktemp); echo "{\"requestedAdditionalDuration\": ${2:?seconds}}" > "$body"
            call POST "/sessions/$(id)/extend" "$body"; rm -f "$body" ;;
    delete) call DELETE "/sessions/$(id)" ;;
    *) sed -n '2,7p' "$0"; exit 1 ;;
esac
