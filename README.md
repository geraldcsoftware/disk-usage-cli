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

In development. No releases yet. Progress is tracked on the [wayfinder map](https://github.com/geraldcsoftware/disk-usage-cli/issues/1), which links the design specification and the milestone issues for the staged prereleases v0.1.0 to v0.4.0.

## Licence

MIT. See [LICENSE](LICENSE).

## Requirements

- macOS 26 (Tahoe) or later on Apple silicon or Intel. The safe deletion path relies on `unlinkat(2)` flags that earlier kernels reject.
- Go 1.27 to build from source.
