#!/usr/bin/env bash
# Runs the real Go Web server locally without API keys or an Eino/provider call.
set -euo pipefail
temp="$(mktemp -d)"
pid=""
cleanup() {
  if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  rm -rf "$temp"
}
trap cleanup EXIT
go build -o "$temp/aegis-web" ./cmd/aegis-web
url="http://127.0.0.1:18687"
start() {
  env -u AEGIS_WEB_API_TOKEN -u AEGIS_CREDENTIAL_MASTER_KEY "$temp/aegis-web" \
    -dev -listen 127.0.0.1:18687 \
    -store "$temp/runs" -credential-store "$temp/secure/credentials.enc" \
    -evidence-store "$temp/evidence" >"$temp/server.log" 2>&1 &
  pid="$!"
  for i in $(seq 1 80); do
    if curl -fsS --max-time 1 "$url/api/v1/research/runs" >/dev/null 2>&1; then return 0; fi
    if ! kill -0 "$pid" 2>/dev/null; then cat "$temp/server.log"; return 1; fi
    sleep .1
  done
  cat "$temp/server.log"
  return 1
}
start
status="$(curl -fsS "$url/api/v1/settings/credentials")"
printf '%s' "$status" | python3 -c 'import json,sys; v=json.load(sys.stdin); assert v["editable"] is True and v["configured"]["DEEPSEEK_API_KEY"] is False'
secret="synthetic-key-for-loopback-smoke-no-provider-call"
result="$(curl -fsS -X PUT "$url/api/v1/settings/credentials" -H 'Content-Type: application/json' -d "{\"changes\":{\"DEEPSEEK_API_KEY\":\"$secret\"}}")"
if [[ "$result" == *"$secret"* ]]; then echo "credential echoed" >&2; exit 1; fi
printf '%s' "$result" | python3 -c 'import json,sys; assert json.load(sys.stdin)["configured"]["DEEPSEEK_API_KEY"] is True'
test -f "$temp/secure/credentials.enc"
test -f "$temp/secure/.local-dev-master.key"
if grep -aqF "$secret" "$temp/secure/credentials.enc"; then echo "plaintext credential leaked" >&2; exit 1; fi
[[ "$(stat -c '%a' "$temp/secure/credentials.enc")" == "600" ]]
[[ "$(stat -c '%a' "$temp/secure/.local-dev-master.key")" == "600" ]]
kill "$pid"; wait "$pid" 2>/dev/null || true; pid=""
start
curl -fsS "$url/api/v1/settings/credentials" | python3 -c 'import json,sys; assert json.load(sys.stdin)["configured"]["DEEPSEEK_API_KEY"] is True'
kill "$pid"; wait "$pid" 2>/dev/null || true; pid=""
if "$temp/aegis-web" -dev -listen 0.0.0.0:18687 -store "$temp/deny-runs" -credential-store "$temp/secure/credentials.enc" >"$temp/rejected.log" 2>&1; then
  echo "dev mode accepted nonloopback listen address" >&2; exit 1
fi
grep -q -- "-dev requires literal loopback" "$temp/rejected.log"
echo "Local Go HTTP dev smoke passed (no external provider calls)"
