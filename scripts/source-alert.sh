# Written by AI on 2026.11.06 by model gpt-6-astra.
# One alert per outage, with recovery after the next successful read.
set -euo pipefail
state="${GOHOME_ALERT_STATE:-/var/lib/gohome-source-alert}"
mode="${1:-}"
now=$(date +%s)

send() {
  local body
  body=$(jq -n --arg problem "$1" '{source: "gohome-source-alert on gohome", problem: $problem, failed_steps: [], likely_fix: "Inspect source errors, connectivity and authentication. Recovery and TEST messages need no action.", logs: "journalctl -u gohome -u gohome-source-alert -n 100"}')
  # shellcheck disable=SC1090
  . "$CREDENTIALS_DIRECTORY/alert-webhook"
  printf 'header = "Authorization: Bearer %s"\n' "$ALERT_WEBHOOK_KEY" |
    curl --silent --show-error --fail --output /dev/null --config - \
      --connect-timeout 15 --max-time 30 \
      --header 'Content-Type: application/json' --data "$body" "$ALERT_WEBHOOK_URL"
  echo "$1: sent to Grok Bot"
}

transition() {
  local id="$1" current="$2" message="$3" previous=healthy
  [ ! -f "$state/$id-status" ] || previous=$(cat "$state/$id-status")
  if [ "$mode" = --dry-run ]; then
    echo "$id: $current ($message)"
    return
  fi
  if [ "$current" != "$previous" ]; then
    if [ "$current" = healthy ]; then send "RECOVERED: $message"; else send "$message"; fi
    printf '%s\n' "$current" > "$state/$id-status"
  fi
}

if [ "$mode" = --test ]; then
  send "TEST: GoHome host alert delivery test. No action needed."
  exit 0
fi
[ "$mode" = --dry-run ] || mkdir -p "$state"
# Every gohome to watch: this host's, then each other gohome whose plugins this host scrapes.
check_registry() {
  local addr="$1" registry_id="$2" where="$3"
  if registry=$(GOHOME_GRPC_ADDR="$addr" timeout 30 "${GOHOME_CLI:-gohome-cli}" call --data '{}' gohome.registry.v1.Registry/ListPlugins) &&
    jq -e '.plugins | type == "array"' <<< "$registry" >/dev/null; then
    transition "$registry_id" healthy "The plugin registry of $where is responding again."
    [ "$mode" = --dry-run ] || printf '0\n' > "$state/$registry_id-first-failure"
    while IFS= read -r plugin; do
      id=$(jq -r '.pluginId' <<< "$plugin")
      # Dashboard-only plugins have healthy status without a source timestamp.
      if jq -e '.status == "HEALTHY" and .lastSuccess == null' <<< "$plugin" >/dev/null; then
        continue
      fi
      first_seen=$now
      [ ! -f "$state/$id-first-seen" ] || first_seen=$(cat "$state/$id-first-seen")
      last_read=0
      [ ! -f "$state/$id-last-read" ] || last_read=$(cat "$state/$id-last-read")
      reported=$(jq -r '(.lastSuccess // "") | if . == "" then 0 else sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601 end' <<< "$plugin")
      [ "$reported" -le "$last_read" ] || last_read=$reported
      if [ "$mode" != --dry-run ]; then
        printf '%s\n' "$first_seen" > "$state/$id-first-seen"
        printf '%s\n' "$last_read" > "$state/$id-last-read"
      fi
      reference=$last_read
      [ "$reference" -ne 0 ] || reference=$first_seen
      if [ $((now - reference)) -lt 21600 ]; then
        [ "$mode" != --dry-run ] || echo "$id: no alert (less than six hours without a successful read)"
        transition "$id" healthy "$id is reading successfully again."
        continue
      fi
      transition "$id" failed "$id has had no successful read for at least six hours. Last reference: $(jq -nr --argjson t "$reference" '$t | todateiso8601')."
    done < <(jq -c '.plugins[]' <<< "$registry")
  else
    first_failure=0
    [ ! -f "$state/$registry_id-first-failure" ] || first_failure=$(cat "$state/$registry_id-first-failure")
    if [ "$first_failure" -eq 0 ]; then
      first_failure=$now
      [ "$mode" = --dry-run ] || printf '%s\n' "$now" > "$state/$registry_id-first-failure"
    fi
    echo "plugin registry of $where unavailable"
    if [ $((now - first_failure)) -ge 900 ]; then
      transition "$registry_id" failed "The plugin registry of $where has not answered for at least 15 minutes. Its sources are not being monitored."
    fi
  fi
}

check_registry "$GOHOME_GRPC_ADDR" registry "the gohome on this host"
for addr in ${GOHOME_REMOTE_REGISTRIES:-}; do
  check_registry "$addr" "registry-$addr" "the gohome at $addr"
done
# Publish only after the check and any required notifications have succeeded.
if [ "$mode" != --dry-run ]; then
  printf '%s\n' "$now" > "$GOHOME_ALERT_HEARTBEAT.tmp"
  mv "$GOHOME_ALERT_HEARTBEAT.tmp" "$GOHOME_ALERT_HEARTBEAT"
fi
