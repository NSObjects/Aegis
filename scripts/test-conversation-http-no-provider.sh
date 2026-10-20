#!/usr/bin/env bash
# Non-paid real-process Conversation HTTP smoke: invalid config prevents provider setup.
set -euo pipefail
dir="$(mktemp -d)"
chmod 700 "$dir"
trap 'kill "${pid:-0}" 2>/dev/null || true; wait "${pid:-0}" 2>/dev/null || true; rm -rf "$dir"' EXIT
unset DEEPSEEK_API_KEY TUSHARE_TOKEN TAVILY_API_KEY AEGIS_WEB_API_TOKEN AEGIS_CREDENTIAL_MASTER_KEY || true
go build -o "$dir/aegis-web" ./cmd/aegis-web
"$dir/aegis-web" -dev -portfolio-review=false -listen 127.0.0.1:18687 \
 -config "$dir/nonexistent-config.yaml" -store "$dir/runs" \
 -credential-store "$dir/credentials.enc" -evidence-store "$dir/evidence" >"$dir/server.log" 2>&1 &
pid=$!
ready=0
for i in $(seq 1 100); do
 if curl -fsS --max-time 1 http://127.0.0.1:18687/api/v1/research/runs > /dev/null 2>&1; then ready=1; break; fi
 if ! kill -0 "$pid" 2>/dev/null; then echo "Go service failed to start"; exit 1; fi
 sleep .1
done
test "$ready" -eq 1
code=$(curl -sS --max-time 10 -o "$dir/start.json" -w '%{http_code}' \
 -H 'Content-Type: application/json' \
 --data '{"message":"确认研究","start_symbol":"600519.SH","confirm_start":true}' \
 http://127.0.0.1:18687/api/v1/conversation/query)
test "$code" = 200
id=$(jq -er '.run_id | select(test("^[a-f0-9]{32}$"))' "$dir/start.json")
curl -fsS --max-time 10 "http://127.0.0.1:18687/api/v1/research/runs/$id" >"$dir/run.json"
jq -e --arg id "$id" '.run_id==$id and .symbol=="600519.SH" and .mode=="DECISION"' "$dir/run.json" >/dev/null
echo "PASS: real Go HTTP Conversation creates a persisted DECISION Run (providers disabled)"
