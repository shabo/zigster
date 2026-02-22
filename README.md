# Zigster

Real-time hardware temperature monitor with a retro DOS aesthetic. Built with [BubbleTea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).

<img width="3450" height="1570" alt="Zigster live monitor" src="https://github.com/user-attachments/assets/7ae8e492-da1d-490b-9503-96d8e793125d" />

## Features

**Live monitoring** -- polls every second, auto-discovers all sensors, one compact line per sensor with sparkline history charts. Color-coded thresholds (green/yellow/orange/red) and minute tick marks on sparklines.

**Dynamic sensor discovery** -- detects CPU, GPU, NVMe, SATA HDD, WiFi, PCH, and any other hwmon sensor automatically. No hardcoded sensor paths. Stable sort order so sensors never jump around between polls.

**Multiple data sources** -- parses `sensors -j` (lm-sensors), `nvidia-smi` (NVIDIA GPU with slowdown/shutdown thresholds), `smartctl` (SATA drive temps), and drivetemp hwmon.

**Persistent history** -- writes CSV data to `~/.sensors-data/` with daily rotation. Every poll is recorded, giving you a full thermal log.

**History viewer** -- scrub through saved data with `[`/`]` day navigation and left/right time cursor. Sparkline windows show temperature context around the selected time.

**Stress testing** -- built-in stress tests for individual components or everything at once. CPU via stress-ng, GPU via glmark2, NVMe/disk via fio, network via iperf3/ping.

## Requirements

- Go 1.21+
- `lm-sensors` (the `sensors` command)
- Optional: `nvidia-smi`, `smartmontools`, `stress-ng`, `fio`, `glmark2`, `iperf3`

## Install

```
git clone https://github.com/shabo/zigster.git
cd zigster
make build
```

## Usage

```
make              # show all available targets
make start        # build and run live monitor
make history      # browse saved temperature history
```

### Stress testing

```
make stress-cpu               # stress all CPU cores (default 60s)
make stress-gpu               # stress NVIDIA GPU
make stress-nvme              # stress NVMe SSD (random 4K I/O)
make stress-disk              # stress SATA HDD (sequential I/O)
make stress-wifi              # stress network adapter
make stress-all               # stress everything at once
make stress-cpu DURATION=30s  # custom duration
```

### Keyboard shortcuts (live monitor)

| Key       | Action               |
|-----------|----------------------|
| `q`       | Quit                 |
| `p`       | Pause/resume polling |
| `Up/Down` | Scroll sensor list   |

### Keyboard shortcuts (history viewer)

| Key         | Action                    |
|-------------|---------------------------|
| `q`         | Quit                      |
| `[` / `]`   | Previous / next day       |
| `Left/Right`| Scrub through time        |
| `Up/Down`   | Scroll sensor list        |

## How it works

The monitor runs a 1-second poll loop that:

1. Calls `sensors -j` and parses the JSON output for all hwmon chips
2. Queries `nvidia-smi` for GPU temperatures (if available)
3. Reads drivetemp hwmon or falls back to `smartctl` for SATA drives
4. Maps chip names to friendly component labels (~28 known patterns)
5. Maintains a 600-point ring buffer per sensor (10 minutes of history)
6. Appends every reading to a daily CSV file in `~/.sensors-data/`
7. Renders a compact TUI with sparkline charts, color thresholds, and stable ordering

## Project structure

```
cmd/sensors/
  main.go                Thin entrypoint, dispatches to monitor/viewer/stress

internal/
  sensor/                Dynamic hardware sensor discovery
    reading.go             Reading type and Key() method
    parser.go              JSON + text fallback parsers for lm-sensors
    sources.go             NVIDIA GPU (nvidia-smi), SATA drives (smartctl/drivetemp)
    identity.go            Chip-to-component friendly name mapping (~28 patterns)
    parser_test.go         Parser and identity tests

  history/               Per-sensor temperature history
    history.go             Ring buffer with min/peak/avg, timestamped points
    history_test.go        Buffer capacity, LastN, LastNPoints tests

  chart/                 Sparkline rendering
    chart.go               Color-coded sparklines, minute ticks, threshold scale
    chart_test.go          Sparkline and tick mark tests

  store/                 Persistent CSV storage
    store.go               Daily rotation, load/list/query, ~/.sensors-data/
    store_test.go          Round-trip write/read test

  monitor/               Live monitoring TUI
    monitor.go             BubbleTea model, polling, panel rendering

  viewer/                History browser TUI
    viewer.go              Time scrubber, day navigation, sparkline windows

  stress/                Stress testing
    stress.go              CPU/GPU/NVMe/disk/WiFi/all stress runners

Makefile                 Build, run, stress, test, clean targets with help menu
```

## License

MIT
