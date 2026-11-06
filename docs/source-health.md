# Source health

Written by AI on 2026.11.06 by model gpt-6-astra.

Registry health follows collection, not plugin construction. Before the first
read it is DEGRADED with "Awaiting first source read". All of a plugin's `*_scrape_success` gauges must be 1,
collection must have completed within one minute, and the oldest reported
`*_last_success_timestamp_seconds` must be within 15 minutes. This accommodates
the existing five-minute Growatt and ten-minute Daikin fetch intervals.
Configuration failures remain ERROR; dashboard-only plugins have no source.

`ListPlugins` and `DescribePlugin` expose `last_success` as a protobuf Timestamp.
It is absent before any successful read and does not advance on a failure.
Plugins without a last-success gauge use the successful collection time; cached
collectors must supply the actual fetch timestamp. RPC reads do not update this
collection-based health. Source timestamps remain separate from fetch timestamps.

Collectors must emit no source values on read failure, and omit fields absent
from a successful response. Diagnostic success and last-success metrics remain.
The core observes these existing metrics; Weheat, Roborock and new plugins need
no additional health interface. Collector instances are shared and serialized
across HTTP endpoints.

VictoriaMetrics scrapes `/metrics/core` and `/metrics/<plugin_id>` independently.
One timed-out source therefore cannot discard another source's scrape. The
aggregate `/metrics` endpoint remains available for manual inspection and still
waits for all sources. There is no background polling loop.

Growatt resolves the requested plant through `device/list` and reads its single
MIN inverter. Multiple inverters and unsupported device types return an explicit
error rather than choosing or summing account devices. Power older than 15 minutes
is absent from metrics and the optional RPC power field; energy totals and the
source timestamp remain available. This covers overnight inverter inactivity.

OAuth refresh is scheduled before the token's final minute, preventing the
30-second unavailable-token interval seen before each Tado refresh.

Read-only live failure/recovery probe (provide a sensor URL and an unreachable URL):

```sh
AIRGRADIENT_LIVE_URL=... AIRGRADIENT_DOWN_URL=... devenv shell -- \
  go test ./plugins/airgradient -run TestLiveSourceFailureRemovesReadings -v
```
