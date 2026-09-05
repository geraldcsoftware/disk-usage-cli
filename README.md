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

In design. No releases yet.

## Requirements

- macOS 13 or later on Apple silicon or Intel.
- Go 1.27 to build from source.
