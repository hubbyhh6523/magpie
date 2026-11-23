#!/bin/sh
# CodeArts gateway + Magpie, one container (blitz / Docker).
#
# Starts the vendored CodeArts Gateway v3 on 127.0.0.1:16211 first, wires
# it into Magpie as the `codearts` provider, then execs `magpie serve`.
# Without CODEARTS_AK / CODEARTS_SK the gateway is skipped and Magpie
# starts alone, so a deploy with no credentials yet still comes up.
#
# Env vars (set them in the blitz dashboard; never bake them into the image):
#   CODEARTS_AK        Huawei Cloud access key id (required for the gateway)
#   CODEARTS_SK        Huawei Cloud secret access key (required for the gateway)
#   CODEARTS_GW_KEY    gateway API key Magpie uses (optional: generated once
#                      and kept in /config/codearts-gateway/gw-key if unset)
#   CODEARTS_GW_SECRET store encryption secret (optional: generated once and
#                      kept in /config/codearts-gateway/gw-secret if unset)
#   CODEARTS_GATEWAY   upstream base URL (optional, default is the CodeArts
#                      MaaS endpoint the gateway was built with)
#
# State that must survive redeploys lives in /config/codearts-gateway/:
# accounts.json (gateway store), logs.db, usage.db, gw-key, gw-secret.
# The Magpie provider itself lives in Magpie's own config, /config/magpie/.

set -u

GW_DIR=/config/codearts-gateway
GW_ADDR=127.0.0.1:16211
GW_URL="http://127.0.0.1:16211/v1"

log() { echo "[entrypoint] $*"; }

# Anything but `serve` runs straight through (healthcheck, version, ...).
if [ "${1:-serve}" != "serve" ]; then
    exec /magpie "$@"
fi

if [ -z "${CODEARTS_AK:-}" ] || [ -z "${CODEARTS_SK:-}" ]; then
    log "CODEARTS_AK/CODEARTS_SK not set; starting Magpie without the CodeArts gateway"
    exec /magpie "$@"
fi

mkdir -p "$GW_DIR" || { log "cannot create $GW_DIR"; exec /magpie "$@"; }
chmod 700 "$GW_DIR" 2>/dev/null || true

# Gateway API key: env var wins, else the stored one, else generate once.
if [ -n "${CODEARTS_GW_KEY:-}" ]; then
    GW_KEY="$CODEARTS_GW_KEY"
elif [ -s "$GW_DIR/gw-key" ]; then
    GW_KEY="$(cat "$GW_DIR/gw-key")"
else
    GW_KEY="$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')"
    printf '%s' "$GW_KEY" > "$GW_DIR/gw-key"
    chmod 600 "$GW_DIR/gw-key"
    log "generated a new gateway key (stored in $GW_DIR/gw-key)"
fi

# Store encryption secret: same rule as the key.
if [ -n "${CODEARTS_GW_SECRET:-}" ]; then
    GW_SECRET="$CODEARTS_GW_SECRET"
elif [ -s "$GW_DIR/gw-secret" ]; then
    GW_SECRET="$(cat "$GW_DIR/gw-secret")"
else
    GW_SECRET="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
    printf '%s' "$GW_SECRET" > "$GW_DIR/gw-secret"
    chmod 600 "$GW_DIR/gw-secret"
    log "generated a new store secret (stored in $GW_DIR/gw-secret)"
fi

log "starting CodeArts gateway on $GW_ADDR"
/usr/local/bin/codearts-gateway \
    --listen "$GW_ADDR" \
    --secret "$GW_SECRET" \
    --gw-key "$GW_KEY" \
    --store "$GW_DIR/accounts.json" \
    --codearts-ak "$CODEARTS_AK" \
    --codearts-sk "$CODEARTS_SK" \
    ${CODEARTS_GATEWAY:+--codearts-gateway "$CODEARTS_GATEWAY"} \
    --logs-db "$GW_DIR/logs.db" \
    --usage-db "$GW_DIR/usage.db" \
    >> "$GW_DIR/gateway.log" 2>&1 &
GW_PID=$!

# Wait for the gateway to answer /v1/models with our key.
UP=0
i=0
while [ "$i" -lt 60 ]; do
    if curl -fsS -o /dev/null -H "Authorization: Bearer $GW_KEY" \
        "http://$GW_ADDR/v1/models" 2>/dev/null; then
        UP=1
        break
    fi
    kill -0 "$GW_PID" 2>/dev/null || break
    i=$((i + 1))
    sleep 0.5
done

if [ "$UP" = 1 ]; then
    log "gateway is up; wiring the codearts provider"
    # Chats need at least one account in the pool (signing itself uses the
    # CLI AK/SK, but the pool picks the account to send on). The store
    # persists it in accounts.json, so this only happens on first start.
    ACCOUNTS="$(curl -fsS -H "Authorization: Bearer $GW_KEY" \
        "http://$GW_ADDR/admin/api/accounts" 2>/dev/null || echo '[]')"
    case "$ACCOUNTS" in
        *'"id"'*) log "gateway already has an account" ;;
        *)
            if curl -fsS -o /dev/null -X POST \
                -H "Authorization: Bearer $GW_KEY" \
                -H 'Content-Type: application/json' \
                -d '{"name":"codearts-primary","credential":"signed-by-gateway"}' \
                "http://$GW_ADDR/admin/api/accounts" 2>/dev/null; then
                log "created the gateway account"
            else
                log "failed to create the gateway account"
            fi
            ;;
    esac
    if /magpie provider codearts >/dev/null 2>&1; then
        # Already there: point it at the in-container gateway, key included.
        /magpie provider set codearts "chat=$GW_URL" >/dev/null 2>&1 || log "provider set failed"
        /magpie provider key codearts "$GW_KEY" >/dev/null 2>&1 || log "provider key failed"
    else
        /magpie provider add codearts "chat=$GW_URL" "key=$GW_KEY" >/dev/null 2>&1 \
            || log "provider add failed"
    fi
    # Fetch the model list (best effort: a chat upstream fault does not
    # stop the container; the list is the gateway's local fallback).
    /magpie provider refresh codearts >/dev/null 2>&1 \
        || log "codearts model refresh failed (gateway is up; see $GW_DIR/gateway.log)"
else
    log "gateway did not answer in 30s (pid $GW_PID may have exited; see $GW_DIR/gateway.log); starting Magpie anyway"
fi

exec /magpie "$@"
