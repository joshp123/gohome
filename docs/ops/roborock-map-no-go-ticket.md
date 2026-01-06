# Ticket: Roborock no-go zone map overlay

## Problem
We can render map + rooms + trace, but no-go zones are not shown. We need to locate the map block containing no-go data and render it.

## Current observations (2026-01-06)
On gohome host, `gohome roborock map-blocks` outputs:

```
block 2 len=161824
block 1 len=12
block 8 len=12
block 3 len=1720
block 18 len=430
block 9 len=64
block 21 len=18
block 10 len=0
block 12 len=0
block 31 len=0
block 32 len=12
block 15 len=0
block 16 len=0
block 17 len=161824
block 19 len=0
block 22 len=16
block 39 len=280
block 28 len=0
block 30 len=0
block 24 len=32
block 25 len=0
block 29 len=0
block 26 len=4
block 33 len=4
block 34 len=70
block 41 len=0
block 1024 len=20
```

Unknown block types: 3, 18, 9, 21, 22, 39, 24, 34, 1024 (and others).

## Plan
1. Add a CLI flag to dump a specific block payload (hex) so we can diff.
2. Capture baseline map blocks.
3. Add a no-go zone in the Roborock app.
4. Capture map blocks again.
5. Identify the block with changed payload and implement parsing + rendering.

## Notes
- Map block parsing lives in `plugins/roborock/map_parser.go`.
- `gohome roborock map-blocks` is already wired on the host.
