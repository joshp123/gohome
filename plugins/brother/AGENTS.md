# Brother Printer Plugin

Read-only maintenance and consumable telemetry from a Brother network printer's local HTTP CSV endpoint (`/etc/mnt_info.csv`).

## RPCs

- `gohome.plugins.brother.v1.BrotherService/GetPrinterStatus`: fetches current toner/drum life, page counters, duplex count, paper jams, and recent printer errors.

## Metrics

- `gohome_brother_toner_remaining_percent`
- `gohome_brother_drum_remaining_percent`
- `gohome_brother_pages_total`
- `gohome_brother_pages_duplex_total`
- `gohome_brother_average_coverage_percent`
- `gohome_brother_paper_jams_total`
- `gohome_brother_paper_jams_by_location_total{location="tray1|inside|rear|duplex"}`
- `gohome_brother_consumable_replacements_total{part="toner|drum"}`
- `gohome_brother_info{model,serial,node_name,firmware}`
- `gohome_brother_scrape_success`
