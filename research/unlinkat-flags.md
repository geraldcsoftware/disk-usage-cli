# Minimum macOS for `AT_SYMLINK_NOFOLLOW_ANY` and `AT_RESOLVE_BENEATH`

Resolves issue #4. Research carried out on 05/09/2026 on macOS 26.6.2 (Darwin 25.6.0, kernel `xnu-12377.161.14~5`, arm64) with Command Line Tools SDK macOS 26.4, Go 1.27.0 and Homebrew 6.0.21.

## Question

What is the minimum macOS version on which `unlinkat(2)` and `fstatat(2)` accept `AT_SYMLINK_NOFOLLOW_ANY` (0x0800) and `AT_RESOLVE_BENEATH` (0x2000), and what does an older kernel return? Does `golang.org/x/sys/unix` define the constants for darwin? Which Homebrew `depends_on macos:` symbol matches the resulting minimum?

## Answer in one paragraph

The minimum is macOS 26.0 (Tahoe). `AT_RESOLVE_BENEATH` first appears in the kernel shipped with macOS 26.0 (xnu-12377.1.9) and is absent from every macOS 15 kernel up to 15.6. `AT_SYMLINK_NOFOLLOW_ANY` is older (header from macOS 12, honoured by `fstatat` from macOS 12) but `unlinkat` only started accepting it in macOS 14.0; macOS 13 rejects it. Every kernel examined validates the `flag` argument against an explicit allow list before any path lookup and returns `EINVAL` for a bit outside the list, so on macOS 13, 14 and 15 the call `unlinkat(fd, path, AT_SYMLINK_NOFOLLOW_ANY | AT_RESOLVE_BENEATH)` fails closed with `EINVAL` and deletes nothing. `golang.org/x/sys/unix` v0.47.0 (latest tag) and master do not define either constant for darwin, so spec section 19 stands. The Homebrew symbol is `:tahoe`, giving `depends_on macos: :tahoe`.

## Sub question 1: which SDK and kernel introduced each flag

### Header introduction (`bsd/sys/fcntl.h` in apple-oss-distributions/xnu, mapped to macOS releases through the `xnu` submodule pointer in apple-oss-distributions/distribution-macOS)

| macOS release | distribution-macOS tag | xnu tag (submodule sha match) | `AT_SYMLINK_NOFOLLOW_ANY` 0x0800 | `AT_RESOLVE_BENEATH` 0x2000 | `O_RESOLVE_BENEATH` (open only) |
|---|---|---|---|---|---|
| 10.15.x | not checked | xnu-6153.121.1 | absent | absent | absent |
| 11.0 | not resolvable (no `macos-110` tag) | xnu-7195.50.7.100.1 | absent | absent | absent |
| 12.3 | macos-123 | xnu-8020.101.4 | line 205 | absent | absent |
| 13.0 | macos-130 | xnu-8792.41.9 | line 215 | absent | absent |
| 13.3 | not resolved | xnu-8796.101.5 | line 215 | absent | absent |
| 13.5 | macos-135 | xnu-8796.141.3 | present | absent | absent |
| 14.0 | macos-140 | xnu-10002.1.13 | line 215 | absent | absent |
| 15.0 | macos-150 | xnu-11215.1.10 | line 215 | absent | line 130 (defined only) |
| 15.4 | not resolved | xnu-11417.101.15 | line 217 | absent | line 130 |
| 15.6 | macos-156 | xnu-11417.140.69 | line 217 | absent | line 130 |
| 26.0 | macos-260 | xnu-12377.1.9 | line 215 | line 216 | line 130, enforced |

The submodule sha at each `distribution-macOS` tag matched the listed xnu tag exactly: `macos-260` → `f6217f89…` = xnu-12377.1.9; `macos-156` → `43a90889…` = xnu-11417.140.69; `macos-150` → `8d741a5d…` = xnu-11215.1.10; `macos-146` → xnu-10063.141.1; `macos-140` → `1031c584…` = xnu-10002.1.13; `macos-135` → xnu-8796.141.3; `macos-130` → `5c2921b0…` = xnu-8792.41.9; `macos-123` → `e7776783…` = xnu-8020.101.4.

The SDK on this machine (`/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk`, `SDKSettings.json` `Version` 26.4) defines both flags in `usr/include/sys/fcntl.h` at lines 184 and 185 inside `#if __DARWIN_C_LEVEL >= __DARWIN_C_FULL`, with `AT_NODELETEBUSY` 0x4000 and `AT_UNIQUE` 0x8000 following. The defines carry no `__API_AVAILABLE` annotation, so the SDK header alone does not state a minimum; the kernel source is the authority. `RENAME_RESOLVE_BENEATH` 0x20 is in `usr/include/sys/stdio.h` line 40 and `O_RESOLVE_BENEATH` 0x1000 in `usr/include/sys/fcntl.h` line 128. `<fcntl.h>` includes `<sys/fcntl.h>`; there is no separate definition.

### Kernel acceptance (`bsd/vfs/vfs_syscalls.c`, the `uap->flag & ~(…)` allow list at the top of each syscall entry point)

`unlinkat`:

| xnu tag (macOS) | line | accepted flags |
|---|---|---|
| xnu-7195.141.2 (11.x) | 5754 | `AT_REMOVEDIR \| AT_REMOVEDIR_DATALESS` |
| xnu-8020.101.4 (12.3) | 5765 | `AT_REMOVEDIR \| AT_REMOVEDIR_DATALESS` |
| xnu-8792.41.9 (13.0) | 6230 | `AT_REMOVEDIR \| AT_REMOVEDIR_DATALESS` |
| xnu-8796.141.3 (13.5) | 6297 | `AT_REMOVEDIR \| AT_REMOVEDIR_DATALESS` |
| xnu-10002.1.13 (14.0) | 6294 | `AT_REMOVEDIR \| AT_REMOVEDIR_DATALESS \| AT_SYMLINK_NOFOLLOW_ANY` |
| xnu-11215.1.10 (15.0) | 6385 | `AT_REMOVEDIR \| AT_REMOVEDIR_DATALESS \| AT_SYMLINK_NOFOLLOW_ANY` |
| xnu-12377.1.9 (26.0) | 6730 | `AT_REMOVEDIR \| AT_REMOVEDIR_DATALESS \| AT_SYMLINK_NOFOLLOW_ANY \| AT_SYSTEM_DISCARDED \| AT_RESOLVE_BENEATH \| AT_NODELETEBUSY` |

`fstatat` and `fstatat64`:

| xnu tag (macOS) | lines | accepted flags |
|---|---|---|
| xnu-7195.141.2 (11.x) | 6649, 6661 | `AT_SYMLINK_NOFOLLOW \| AT_REALDEV \| AT_FDONLY` |
| xnu-8020.101.4 (12.3) | 6665, 6677 | adds `AT_SYMLINK_NOFOLLOW_ANY` |
| xnu-8792.41.9 (13.0) through xnu-11215.1.10 (15.0) | 7130 and 7142; 7194 and 7206; 7200 and 7212; 7291 and 7303 | unchanged |
| xnu-12377.1.9 (26.0) | 7658, 7670 | adds `AT_RESOLVE_BENEATH` |

`openat` with `O_RESOLVE_BENEATH`: the constant is defined from xnu-11215.1.10 (macOS 15.0) `bsd/sys/fcntl.h` line 130 but neither `bsd/vfs/vfs_vnops.c` nor `bsd/vfs/vfs_lookup.c` at that tag references `RESOLVE_BENEATH`, so macOS 15 defines the bit without enforcing it. Enforcement appears in xnu-12377.1.9: `vfs_vnops.c` lines 451 and 577 set `NAMEI_RESOLVE_BENEATH`, and `vfs_lookup.c` lines 510 to 524 and 1397 return `ENOTCAPABLE` for absolute paths, `..` escapes and symlinks that leave the start directory. Do not rely on `O_RESOLVE_BENEATH` on macOS 15.

`renameatx_np` with `RENAME_RESOLVE_BENEATH`: `bsd/sys/stdio.h` gains `RENAME_RESOLVE_BENEATH 0x20` at xnu-12377.1.9 line 40 (absent at xnu-11215.1.10, which ends at `RENAME_NOFOLLOW_ANY` line 39). The allow list at `vfs_syscalls.c` line 10242 (26.0) includes it; lines 9622 (13.0), 9652 (14.0) and 9773 (15.0) do not. `RENAME_NOFOLLOW_ANY` is accepted from at least macOS 13.0.

Apple release notes: web searches for `AT_RESOLVE_BENEATH` returned FreeBSD and Linux material only; no Apple release note or developer document naming the flag was found. The finding rests on the xnu source and the local SDK and man pages.

### Local confirmation on this kernel (Darwin 25.6.0, xnu-12377.161.14)

`man 2 unlinkat` and `man 2 fstatat` on this machine document both flags and list `[EINVAL] The value of the flag argument is not valid`, `[ELOOP]` for `AT_SYMLINK_NOFOLLOW_ANY` and `[ENOTCAPABLE]` for `AT_RESOLVE_BENEATH`. A throwaway C program run against a fixture tree (root fd opened `O_DIRECTORY | O_NOFOLLOW`, symlink `link` pointing outside the root) produced:

```
fstatat f1 NOFOLLOW_ANY|RESOLVE_BENEATH                  ok
fstatat link/target NOFOLLOW_ANY                         ELOOP (62)
fstatat link/target RESOLVE_BENEATH                      ENOTCAPABLE (107)
fstatat ../outside/target RESOLVE_BENEATH                ENOTCAPABLE (107)
fstatat f1 flag 0x0100 / 0x1000 / 0x10000                EINVAL (22) each
unlinkat f1 flag 0x0100 (AT_REMOVEDIR_DATALESS, private) ENOTDIR (20)
unlinkat f1 flag 0x1000 (AT_SYSTEM_DISCARDED, private)   ok, file removed
unlinkat f1 flag 0x10000 (undefined)                     EINVAL (22)
unlinkat f1 AT_EACCESS (defined, not for unlinkat)       EINVAL (22)
unlinkat link/target NOFOLLOW_ANY|RESOLVE_BENEATH        ELOOP (62)
unlinkat link/target RESOLVE_BENEATH                     ENOTCAPABLE (107)
unlinkat ../outside/target RESOLVE_BENEATH               ENOTCAPABLE (107)
unlinkat sub REMOVEDIR|NOFOLLOW_ANY|RESOLVE_BENEATH      ok
openat f2 O_NOFOLLOW_ANY|O_RESOLVE_BENEATH               ok
openat link/target O_RESOLVE_BENEATH                     ENOTCAPABLE (107)
renameatx_np NOFOLLOW_ANY|RESOLVE_BENEATH                ok
renameatx_np flag 0x100                                  EINVAL (22)
```

This proves current acceptance only. The two private bits (0x0100, 0x1000; `bsd/sys/fcntl_private.h` lines 78 and 79 at xnu-12377.1.9) are accepted because they sit in the allow list; they are irrelevant to dusk but show that "not in the public header" is not the same as "rejected".

## Sub question 2: what an older kernel returns for an unknown flag bit

`EINVAL`, before any path lookup. Every `unlinkat` and `fstatat` entry point examined (macOS 11 through 26) opens with `if (uap->flag & ~(<allow list>)) return EINVAL;`, so a bit outside the list is rejected without touching the filesystem. On macOS 13 both 0x0800 and 0x2000 are outside the `unlinkat` list; on macOS 14 and 15, 0x2000 is. `fstatat` rejects 0x2000 on every release before 26.0. There is no silent acceptance path for these two bits on any release examined.

Consequence for the design: the combination `AT_SYMLINK_NOFOLLOW_ANY | AT_RESOLVE_BENEATH` fails closed on every macOS older than 26.0. A start up OS version check is therefore not required for safety (invariant 6 cannot be silently weakened). It is still recommended for usability: `dusk doctor` should report the OS version, and the eviction executor should translate `EINVAL` from `unlinkat` or `fstatat` into a clear "requires macOS 26 or later" message rather than journalling a bare errno, since on an unsupported release every unit would otherwise be recorded as `failed` with exit 73.

## Sub question 3: `golang.org/x/sys/unix` coverage for darwin

Not defined. Checked at the latest tag v0.47.0 (commit 9e7e939d, 30/06/2026, the newest version reported by `go list -m -versions golang.org/x/sys`) and at master 613e2570 (31/08/2026):

- `unix/ztypes_darwin_arm64.go` and `ztypes_darwin_amd64.go` lines 691 to 695 define only `AT_FDCWD`, `AT_REMOVEDIR`, `AT_SYMLINK_FOLLOW`, `AT_SYMLINK_NOFOLLOW` and `AT_EACCESS`; the source list is hand maintained in `unix/types_darwin.go` lines 319 to 323.
- `unix/zerrors_darwin_arm64.go` has `O_NOFOLLOW_ANY` (line 1136) and `RENAME_NOFOLLOW_ANY` (line 1176) but nothing matching `RESOLVE_BENEATH` or `AT_SYMLINK_NOFOLLOW_ANY` (grep count 0 at v0.47.0).
- `unix.Unlinkat(dirfd int, path string, flags int)` (`zsyscall_darwin_arm64.go` line 2427) and `unix.Fstatat(fd int, path string, stat *Stat_t, flags int)` (line 2613) pass `flags` through unchanged, so locally defined constants work with the existing wrappers.

Spec section 19 (constants defined in `internal/sys` with their SDK header source noted) remains correct. Source note for the two constants: `MacOSX.sdk/usr/include/sys/fcntl.h` lines 184 and 185, SDK 26.4.

## Sub question 4: Homebrew symbol

`:tahoe`. `Library/Homebrew/macos_version.rb` in Homebrew/brew (master commit 728a3537, 04/09/2026) line 25 maps `tahoe: "26"`, with `sequoia: "15"`, `sonoma: "14"`, `ventura: "13"` below it. The installed Homebrew 6.0.21 carries the same table. The Formula Cookbook (docs.brew.sh/Formula-Cookbook) states: "Top-level `depends_on macos: :sonoma` marks a formula as macOS-only and declares the minimum compatible macOS release", so `depends_on macos: :tahoe` replaces, rather than supplements, the bare `depends_on :macos` in the current spec.

GoReleaser's `brews` section (goreleaser.com/customization/homebrew_formulas) has no field for a macOS version requirement; `dependencies` only takes package names. The line must go through `custom_block`. The Homebrew source comment at the same table notes that Big Sur and macOS x86_64 support are scheduled for removal in or after September 2027, which is a scheduling input for the amd64 build, not a blocker.

## Confidence

High for the minimum version and errno behaviour: both derive from the kernel source at release tagged commits mapped through Apple's own distribution manifest, corroborated by the local SDK, man pages and probe. High for x/sys and Homebrew, read directly from the current repositories. Not established: any Apple release note naming the flags, and the exact 12.x point release that introduced `AT_SYMLINK_NOFOLLOW_ANY` (between 11.x and 12.3; immaterial once the minimum is 26.0).

## Recommended spec amendments

Section 2, Decisions table, first row:

> Language, platform | Go 1.27, macOS 26 (Tahoe) or later, `CGO_ENABLED=0`, arm64 and amd64

Section 3, Symlinks and roots paragraph, append:

> `AT_RESOLVE_BENEATH` is accepted by `unlinkat` and `fstatat` from macOS 26.0 (xnu-12377.1.9); `unlinkat` accepts `AT_SYMLINK_NOFOLLOW_ANY` from macOS 14.0. Every older kernel returns `EINVAL` for an unknown flag bit before any lookup, so the combination fails closed rather than silently.

Section 21, `brews` sentence, replace `depends_on :macos` with:

> `depends_on macos: :tahoe` (emitted through GoReleaser `custom_block`, since `brews` has no macOS version field)

Section 12 step 5 (suggestion, owner decision): the pre delete `fstatat` could also carry `AT_SYMLINK_NOFOLLOW_ANY | AT_RESOLVE_BENEATH` now that the minimum makes them available, so the verification stat and the unlink resolve the path under identical rules.

## Sources

- Apple SDK: `/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk/usr/include/sys/fcntl.h` lines 128, 158, 177 to 187; `usr/include/sys/stdio.h` lines 35 to 40, 53; `SDKSettings.json`.
- Man pages on this machine: `unlinkat(2)`, `fstatat(2)`, `open(2)`, `renameatx_np(2)`.
- apple-oss-distributions/xnu: `bsd/sys/fcntl.h`, `bsd/sys/fcntl_private.h`, `bsd/sys/stdio.h`, `bsd/vfs/vfs_syscalls.c`, `bsd/vfs/vfs_vnops.c`, `bsd/vfs/vfs_lookup.c` at tags xnu-6153.121.1, xnu-7195.50.7.100.1, xnu-7195.141.2, xnu-8020.101.4, xnu-8792.41.9, xnu-8796.101.5, xnu-8796.141.3, xnu-10002.1.13, xnu-11215.1.10, xnu-11417.101.15, xnu-11417.140.69, xnu-12377.1.9.
- apple-oss-distributions/distribution-macOS: `xnu` submodule pointer at tags macos-123, macos-130, macos-135, macos-140, macos-146, macos-150, macos-156, macos-260.
- golang/sys: `unix/types_darwin.go`, `unix/ztypes_darwin_{arm64,amd64}.go`, `unix/zerrors_darwin_{arm64,amd64}.go`, `unix/zsyscall_darwin_arm64.go` at v0.47.0 and master 613e2570.
- Homebrew/brew: `Library/Homebrew/macos_version.rb` at 728a3537; `Library/Homebrew/requirements/macos_requirement.rb` (installed 6.0.21); docs.brew.sh Formula Cookbook.
- GoReleaser documentation: customization/homebrew_formulas.
