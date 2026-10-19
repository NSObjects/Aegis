#!/usr/bin/env bash
# Explicitly gated by integration.yml run_conversation_hosted.
# Exactly one confirmed research POST. NO RETRIES of a paid side effect.
set -euo pipefail
data_dir="$RUNNER_TEMP/aegis-conversation-hosted"
mkdir -p "$data_dir"
chmod 700 "$data_dir"
unset AEGIS_WEB_API_TOKEN AEGIS_CREDENTIAL_MASTER_KEY || true
"$RUNNER_TEMP/aegis-web" -dev -listen 127.0.0.1:18686 \
  -store "$data_dir/runs" -credential-store "$data_dir/credentials.enc" \
  -evidence-store "$data_dir/evidence" >"$data_dir/server.log" 2>&1 &
server_pid=$!
trap 'kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true' EXIT
base=http://127.0.0.1:18686
ready=false
for i in $(seq 1 120); do
  if curl -fsS --max-time 1 "$base/api/v1/research/runs" >/dev/null 2>&1; then ready=true; break; fi
  if ! kill -0 "$server_pid" 2>/dev/null; then echo "::error::Go service exited"; exit 1; fi
  sleep 0.25
done
if [ "$ready" != true ]; then echo "::error::Go service not ready"; exit 1; fi

# One and only one confirmed POST. Never put curl in a retry loop.
http=$(curl -sS --max-time 45 -o "$data_dir/start.json" -w '%{http_code}' \
  -H 'Content-Type: application/json' -X POST \
  --data '{"message":"请研究 600519.SH","start_symbol":"600519.SH","confirm_start":true}' \
  "$base/api/v1/conversation/query") || {
    echo "::error::Conversation HTTP submission failed; do not retry, inspect server-side Run list"
    exit 1
  }
if [ "$http" != 200 ]; then echo "::error::Conversation start HTTP $http; do not retry"; exit 1; fi
run_id=$(jq -er '.run_id | select(test("^[a-f0-9]{32}$"))' "$data_dir/start.json")
session_id=$(jq -er '.session_id | select(test("^[a-f0-9]{32}$"))' "$data_dir/start.json")
jq -e '.symbol=="600519.SH" and .state=="PENDING"' "$data_dir/start.json" >/dev/null
echo "Conversation created one Run: $run_id"
session_http=$(curl -sS --max-time 10 -o "$data_dir/session.json" -w '%{http_code}' "$base/api/v1/conversation/sessions/$session_id")
test "$session_http" = 200
jq -e --arg id "$run_id" '.run_id==$id and .symbol=="600519.SH"' "$data_dir/session.json" >/dev/null

state=""
for i in $(seq 1 150); do
  curl -fsS --max-time 10 "$base/api/v1/research/runs/$run_id" >"$data_dir/run.json"
  state=$(jq -r '.state // empty' "$data_dir/run.json")
  case "$state" in
    COMPLETED|FAILED|CANCELLED) break;;
    PENDING|RUNNING) sleep 10;;
    *) echo "::error::Invalid persisted Run state"; exit 1;;
  esac
done
echo "Conversation research terminal state: $state; Run ID: $run_id"
if [ "$state" != COMPLETED ]; then
  echo "::error::Hosted research did not complete successfully"
  grep 'AEGIS_RUN_DIAGNOSTIC' "$data_dir/server.log" || true
  exit 1
fi
curl -fsS --max-time 20 "$base/api/v1/research/runs/$run_id/artifact" >"$data_dir/artifact.json"
jq -e 'type=="object" and (length>0)' "$data_dir/artifact.json" >/dev/null

# Follow-up may invoke a model, but MUST NOT create another research Run.
# The only read-side reference is the server-owned session_id.
jq -nc --arg id "$session_id" '{message:"这次研究的空头依据是什么？",session_id:$id}' >"$data_dir/followup-request.json"
followup_http=$(curl -sS --max-time 65 -o "$data_dir/followup.json" -w '%{http_code}' \
  -H 'Content-Type: application/json' -X POST \
  --data-binary @"$data_dir/followup-request.json" "$base/api/v1/conversation/query")
if [ "$followup_http" != 200 ]; then echo "::error::Read-only follow-up HTTP $followup_http"; exit 1; fi
jq -e '.answer | type=="string" and length>0' "$data_dir/followup.json" >/dev/null
curl -fsS --max-time 10 "$base/api/v1/research/runs" >"$data_dir/list.json"
jq -e --arg id "$run_id" '.runs | length==1 and .[0].run_id==$id' "$data_dir/list.json" >/dev/null
echo "PASS: one confirmed Conversation Run completed; persisted artifact and read-only follow-up verified."
