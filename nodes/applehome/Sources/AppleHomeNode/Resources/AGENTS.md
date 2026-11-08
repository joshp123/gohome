# Apple Home - Agent Context

## What This Is
Every accessory in the house's Apple Home, read through Apple's HomeKit framework
by a small Mac app on a Mac. It covers accessories gohome can't reach any
  other way without unpairing them from Apple Home, such as a thermostat
thermostat, and everything else Apple Home gives third-party apps: radiator
thermostats, air-quality monitors, lights and shutters.

It runs as a gohome node on a Mac, not in the gohome hub: HomeKit is only
available to signed Mac apps on a Mac signed in to the Apple account that owns the
home.

## Capabilities
- Read climate: temperature, humidity and thermostat state (`GetClimate`)
- Read every accessory, service and characteristic with its value, unit, when it
  was last read and whether the accessory is reachable (`ListAccessories`)

## Limits
- Read-only. The app never writes to an accessory and never changes a home, room,
  scene or automation. Controlling devices through Apple Home needs a separate
  change, made only when the owner asks for it.
- Only current values are reported. The app reads every readable characteristic
  once a minute and takes HomeKit change notifications in between. A value is
  current when the accessory is reachable and the value was read without error in
  the last 3 minutes. Any other value is left out of the metrics and the API, never
  repeated from HomeKit's cache, so a silent sensor shows as a gap.
- No HomePods. HomeKit leaves HomePods, and so their temperature and humidity
  sensors, out of what it gives third-party apps; only Apple's Home app shows them.
- Only works while the Mac is on, logged in, and the app is running.
- Names, rooms and type names are Apple Home's, in the Mac's system language.

## Methods
- `GetClimate`: one reading per accessory with a temperature, humidity or thermostat
  service. Thermostats include current mode (OFF/HEAT/COOL), target mode
  (OFF/HEAT/COOL/AUTO) and target temperature. Each value has its own `*_read_at`,
  set even when the value is left out as not current; a mode that is not current is
  UNSPECIFIED.
- `ListAccessories`: every accessory in every home, with services and
  characteristics. `type` is HomeKit's readable name ("Current Temperature"),
  `type_uuid` the HAP type. A characteristic's value is set only when current.
  `read_at` is when the value last came from the accessory; `read_error` says why
  the last read failed.

## State
Metrics (Prometheus, `/metrics/applehome` on the node; the hub records them).
Accessory metrics carry `accessory_id`, `accessory` (the Apple Home name), `room`
(the Apple Home room), `sensor` (manufacturer and model, for example "IKEA of
Sweden ALPSTUGA air quality monitor"; the Apple Home name when HomeKit gives
neither) and `home` labels.
- `gohome_applehome_scrape_success`
- `gohome_applehome_last_success_timestamp_seconds`
- `gohome_applehome_home_access_granted`
- `gohome_applehome_homes`
- `gohome_applehome_accessories`
- `gohome_applehome_accessory_reachable{category}`
- `gohome_applehome_temperature_celsius`
- `gohome_applehome_humidity_percent`
- `gohome_applehome_thermostat_target_temperature_celsius`
- `gohome_applehome_thermostat_heating` (1 while heating)
- `gohome_applehome_thermostat_target_mode{mode}` (1 for the set mode)
- `gohome_applehome_characteristic_value{service_id,service,service_type,characteristic_id,characteristic,unit}`:
  every numeric or on/off value, for example lights
  (`service_type="Lightbulb", characteristic="Power State"` or `"Brightness"`)
  shutters (`service_type="Window Covering", characteristic="Current Position"`)
  and air quality (`service_type="Air Quality Sensor", characteristic="Carbon
  dioxide Level"` in ppm or `"PM2.5 Density"` in micrograms/m^3).
- `gohome_applehome_characteristic_last_read_timestamp_seconds`, same labels: when
  each readable characteristic last came from the accessory (0 if never). Use
  `time() - ...` to see how long a sensor has been silent.

Every value metric above carries current values only, so it has a gap while a
sensor is unreachable or failing.

## Errors
- `PERMISSION_DENIED`: macOS denied the app access to Apple Home. Allow it in
  System Settings, Privacy & Security, HomeKit, on the Mac.
- `UNAVAILABLE`: the app is waiting for Home access or its first read.
- Registry status ERROR with "HomeKit returned no homes": the Mac is not signed in
  to an Apple account with a home, or the build lacks the HomeKit entitlement.
- An unreachable accessory shows `reachable: false`, per-characteristic
  `read_error` and no values; the plugin stays HEALTHY while other accessories read.

## Required Config
None in `config.pbtxt`. The node app takes launch arguments: `-listenHost`
(default `127.0.0.1`; the Mac's private network address in production), `-grpcPort`
(default 9000) and `-httpPort` (default 8080). Build, signing and running are in
`nodes/applehome/README.md`.
