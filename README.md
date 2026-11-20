# r9000p-fan-manager

A tiny, dependency-free Go tool for fan / thermal control on the Lenovo Legion
R9000P (and similar Legion laptops with the `legion-laptop` kernel module).
One binary, `fan`, with a simple TUI and sub-commands.

English only. Standard library only (no third-party Go modules), so it builds
offline and cross-compiles easily.

## What it does

- **Quiet (night) mode** – set `power-saver` + a gentle fan curve that lowers
  noise at low temperatures while keeping strong cooling at high temperatures.
- **Max fan** for N seconds (e.g. before a heavy task), then restores the
  previous curve/profile automatically.
- **Status** – platform profile, power profile, CPU governor, fan RPM, temps.
- **Set platform profile** – `low-power`, `balanced`, `performance`, `custom`.

## Requirements

- Linux on a Legion laptop with the **LenovoLegionLinux** `legion_laptop` module
  (exposes `/sys/class/hwmon/*/name == legion_hwmon` and
  `/sys/class/platform-profile/.../profile`).
- `power-profiles-daemon` for the `powerprofilesctl` power mode (optional).
- Root for writes (the tool re-execs itself with `sudo -n`; passwordless sudo
  recommended, otherwise run with `sudo`).

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/crayonlu/r9000p-fan-manager/main/install.sh | sudo bash
```

Installs `/usr/local/bin/fan` from the latest GitHub release.

## Usage

```sh
fan                 # interactive TUI
fan status          # print profile / power / governor / fan / temp
fan quiet on        # enter quiet (night) mode
fan quiet off       # restore previous state
fan max 60          # max fan for 60s, then restore
fan profile balanced
```

## Build from source

```sh
# native (on the target machine)
CGO_ENABLED=0 go build -o fan .

# cross-compile for the laptop from any machine
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o fan-linux-amd64 .
```

## Releases

Pushing a tag like `v0.1.0` triggers `.github/workflows/release.yml`, which
cross-compiles `fan-linux-amd64` / `fan-linux-arm64` and attaches them to the
GitHub release. `install.sh` downloads `releases/latest/download/fan-linux-<arch>`.

## Notes

- Quiet mode uses the `custom` thermal profile so the EC follows the written
  fan curve. On this model the GPU fan may briefly stop in `custom` mode; it
  returns to normal when you leave quiet mode.
- All changes are reversible: `quiet off` and the end of `max` restore the
  saved profile and fan curve.
