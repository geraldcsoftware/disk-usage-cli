# dusk

Disk usage keeper for macOS. A command line tool that monitors free space, flags growth before it becomes a problem, and enforces size limits on directories that grow without bound (build caches, package caches, application caches).

## What it does

- Measures container free space on a schedule and tracks warn and critical states with hysteresis.
- Reports the size of configured directories against a maximum size, with growth history.
- Cleans directories that breach their limit, either on request or, where a rule opts in, automatically. Cleanup mode is per rule: oldest first, largest first, or delete all.
- Runs vendor cleanup commands (Homebrew, npm, Docker and others) as explicitly configured argv arrays, never through a shell.
- Notifies through Notification Centre and exposes a status file for a Starship prompt segment.
- Journals every deletion.

## Status

Prerelease. Progress is tracked on the [wayfinder map](https://github.com/geraldcsoftware/disk-usage-cli/issues/1), which links the design specification and the milestone issues for the staged prereleases v0.1.0 to v0.4.0. Prereleases publish to GitHub only; the Homebrew formula arrives with v1.0.0.

## Trying the prerelease

Download the archive for your architecture from the latest prerelease, unpack it, and put `dusk` on your PATH:

```sh
tar -xzf dusk_*_darwin_arm64.tar.gz
install -m 0755 dusk ~/.local/bin/dusk
dusk version
```

Write a config at `~/.config/dusk/config.toml` (see the design specification, section 6), then:

```sh
dusk config validate
dusk check --full
dusk status
dusk report
```

The LaunchAgent arrives in a later prerelease. Until then a cron entry runs the check every thirty minutes:

```
5,35 * * * * $HOME/.local/bin/dusk check >> $HOME/Library/Logs/dusk/cron.log 2>&1
```

Starship reads the status with:

```toml
[custom.dusk]
command = "dusk status --prompt"
when    = true
style   = "bold yellow"
```

## Licence

MIT. See [LICENSE](LICENSE).

## Requirements

- macOS 26 (Tahoe) or later on Apple silicon or Intel. The safe deletion path relies on `unlinkat(2)` flags that earlier kernels reject.
- Go 1.27 to build from source.
