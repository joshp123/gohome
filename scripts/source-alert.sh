# Written by AI on 2026.11.06 by model gpt-6-astra.
# Alerts Grok Bot after six hours without a source read, at most once a day.
set -euo pipefail
state="${GOHOME_ALERT_STATE:-/var/lib/gohome-source-alert}"
mode="${1:-}"
now=$(date +%s)

if [ "$mode" = --test ]; then
  body=$(jq -n '{source: "gohome-source-alert test", problem: "gohome alert test, no action needed", failed_steps: [], likely_fix: "None. This is an authorised webhook delivery test, not a source failure.", logs: "ssh root@gohome journalctl -u gohome-source-alert -n 50"}')
else
  registry=$("${GOHOME_CLI:-gohome-cli}" call --data '{}' gohome.registry.v1.Registry/ListPlugins)
  # Reject a failed/empty RPC response before changing observation state.
  jq -e '.plugins | type == "array"' <<< "$registry" >/dev/null
  [ "$mode" = --dry-run ] || mkdir -p "$state"
  problems='[]'
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
      continue
    fi
    problems=$(jq --argjson plugin "$plugin" --argjson reference "$reference" --argjson last_read "$last_read" \
      '. + [{source: $plugin.pluginId, since: ($reference | todateiso8601), since_basis: (if $last_read == 0 then "first observed; no successful read recorded" else "last successful read" end), health_message: $plugin.healthMessage, logs: "ssh root@gohome journalctl -u gohome -n 100"}]' <<< "$problems")
  done < <(jq -c '.plugins[]' <<< "$registry")
  [ "$(jq length <<< "$problems")" -gt 0 ] || exit 0
  last_alert=0
  [ ! -f "$state/last-alert" ] || last_alert=$(cat "$state/last-alert")
  if [ $((now - last_alert)) -lt 86400 ]; then
    echo "alert already sent within the last day; not resending"
    exit 0
  fi
  body=$(jq -n --argjson problems "$problems" '{source: "gohome-source-alert on gohome", problem: "Home automation sources have had no successful read for at least six hours.", failed_steps: $problems, likely_fix: "Inspect the source errors in the journal, then check connectivity and authentication for each affected source.", logs: "ssh root@gohome journalctl -u gohome -n 100"}')
  if [ "$mode" = --dry-run ]; then
    printf '%s\n' "$body"
    exit 0
  fi
fi

# shellcheck disable=SC1090
. "$CREDENTIALS_DIRECTORY/alert-webhook"
# The bearer key reaches curl through stdin, never argv.
printf 'header = "Authorization: Bearer %s"\n' "$ALERT_WEBHOOK_KEY" \
  | curl --silent --show-error --fail-with-body --config - \
      --connect-timeout 15 --max-time 60 \
      --header 'Content-Type: application/json' --data "$body" "$ALERT_WEBHOOK_URL"
[ "$mode" = --test ] || printf '%s\n' "$now" > "$state/last-alert"
echo "alert sent to Grok Bot"
