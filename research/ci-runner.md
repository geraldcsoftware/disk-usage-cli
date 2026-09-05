# CI runner capabilities: `hdiutil` and the launchd `gui` domain on `macos-latest`

Resolves issue #5. Research carried out on 05/09/2026 from primary sources only: the `actions/runner-images` repository (readme, Packer templates and image build scripts at `main`, commit `148c0a4a`), GitHub Docs, Apple's `launchctl(1)` and `hdiutil(1)` manual pages, Apple Technical Note 2083, and the logs of public GitHub Actions runs from 01/09/2026 to 03/09/2026 that exercised the relevant commands on hosted macOS runners. Nothing was executed on a hosted runner or on the local machine for this note.

## Question

Can the `macos-latest` GitHub Actions runner run the two privileged looking suites in design specification section 20: the ENOSPC suite (`hdiutil create` and `hdiutil attach` of a small sparse APFS image without root, filled to the APFS allocation floor) and the LaunchAgent suite (`launchctl bootstrap gui/$UID` of a transient agent, observing `EPERM` on `~/Library/Mail` under launchd and success interactively)?

## Answer in one paragraph

`macos-latest` resolves today to the macOS 26 arm64 image (macOS 26.5.2, Darwin 25.5.0, image 20260728.0273.1), the job runs as user `runner` (uid 501) with passwordless `sudo` granted through the `admin` group. `hdiutil create` and `hdiutil attach` work for that user without `sudo`; a run on this exact image on 02/09/2026 created and attached a 20 GiB sparse image, and another attached an APFS image which appeared as its own synthesised APFS container disk with the volume beneath it, so the ENOSPC suite runs on CI. The job step runs inside a real Aqua login session: the image enables automatic GUI login for the runner user, and `launchctl managername` printed `Aqua` from a job step on this image on 03/09/2026, so the `gui/501` domain exists and `launchctl bootstrap gui/$UID` is available (hosted runners on macos-14 arm64 bootstrap, kickstart and print agents in `gui/$(id -u)` in green runs from 01/09/2026). What is not established is the TCC half of the LaunchAgent suite: the image build grants Full Disk Access in the system TCC database to `/bin/bash` but not to the hosted compute agent that owns the job process tree, Apple does not document which process TCC credits for a job step, and nothing shows whether `~/Library/Mail` exists on the image at all. The LaunchAgent suite therefore runs on CI with a named workaround: make the `~/Library/Mail` assertion conditional on a pre-check, and run the probe workflow below once at milestone v0.1.0 to settle the TCC facts before v0.3.0 decides which suites gate a pull request.

## Sub question 1: image version, architecture and runner user

### What `macos-latest` resolves to

The `actions/runner-images` readme "Available Images" table at `main` (commit `148c0a4a`, 20/08/2026) lists, verbatim from the rows:

| Image | Architecture | YAML label |
|---|---|---|
| macOS 26 Arm64 | arm64 | `macos-latest`, `macos-26` or `macos-26-xlarge` |
| macOS 26 | x64 | `macos-latest-large`, `macos-26-intel`, `macos-26-large` |
| macOS 15 Arm64 | arm64 | `macos-15`, or `macos-15-xlarge` |
| macOS 14 Arm64 (deprecated, issue #13518) | arm64 | `macos-14` or `macos-14-xlarge` |

`images/macos/macos-26-arm64-Readme.md` at the same commit states: OS Version macOS 26.5.2 (25F84), Kernel Version Darwin 25.5.0, Image Version 20260728.0273.1. The environment table in that readme uses `/Users/runner` as the home directory (for example `ANDROID_HOME /Users/runner/Library/Android/sdk`).

Confirmed against a live run: `basecamp/basecamp-cli` run 33803101317 (03/09/2026, `runs-on: macos-latest`) logged `Image: macos-26-arm64`, `Version: 20260728.0273.1`, runner 2.337.0, Hosted Compute Agent 20260707.563. `radareorg/iaito` run 33815771177 (03/09/2026, `macos-latest`) likewise logged `Image: macos-26-arm64`.

GitHub Docs, "GitHub-hosted runners" (docs.github.com/en/actions/reference/runners/github-hosted-runners), standard runners for public and private repositories: the row carrying `macos-latest` is macOS, 3 (M1) processors, 7 GB memory, 14 GB storage, arm64, labels `macos-latest`, `macos-14`, `macos-15`, `macos-26`, `xcode-27` (public preview). The docs page does not name the version behind `macos-latest`; the runner-images readme is the authority for that.

The readme also warns that "The `-latest` migration process is gradual and happens over 1-2 months" and that workflows using the label "may see changes in the OS version". Recommendation: the gating jobs in `ci.yml` should pin `macos-26` rather than `macos-latest`, and the release job should do the same, so that the probe results stay valid until the pin is deliberately moved.

### Runner user and privileges

Established: the job runs as `runner`, uid 501, home `/Users/runner`. Evidence: the readme environment table above; `GPSBabel/gpsbabel` run 33658832003 lists files created in the job as owned by `runner staff`; `lablup/all-smi` job 99893591496 shows `/Users/runner/Library/LaunchAgents/...` and `gui/501/...` targets; the image build script `images/macos/scripts/build/configure-machine.sh` hardcodes `launchctl bootout gui/501/${term_service}`.

Established: passwordless `sudo`. GitHub Docs (same page): "The Linux and macOS virtual machines both run using passwordless `sudo`."

Inferred: the runner user is a member of `admin`. `configure-machine.sh` grants the passwordless rule by rewriting the `%admin` line of `/etc/sudoers` to `%admin ALL = (ALL) NOPASSWD: ALL` and runs `sudo chown $USER:admin /usr/local/bin` and `/usr/local/share`; passwordless `sudo` for the runner user therefore flows from `admin` membership. No script prints `id` or `dsmemberutil` output, so the probe below records it.

## Sub question 2: `hdiutil` without `sudo`, and whether the image is a separate APFS container

### Manual page

`hdiutil(1)` (Xcode manual page mirror, keith.github.io/xcode-man-pages/hdiutil.1.html): `create -type SPARSE` produces a read/write single file image that grows as it fills; `-fs` accepts `APFS` and "Default is APFS"; `-size` takes `??m` and `??g` suffixes; `attach -nobrowse` hides the volume from Finder and `-owners on|off` controls ownership honouring; `detach` takes a device or mount point and `-force` ignores open files. The page states no root requirement for `create`, `attach` or `detach`. On APFS it states that imaging individual volumes is invalid and the device must be an APFS container or contain one, so an APFS image always carries a container.

### Observed on the current `macos-latest` image, without `sudo`

`openwrt/openwrt` run 33665818599 (02/09/2026), job "Build tools with macos latest", `Image: macos-26-arm64`, executing `openwrt/actions-shared-workflows/.github/workflows/tools.yml` lines 25 to 26:

```
hdiutil create -size 20g -type SPARSE -fs "Case-sensitive HFS+" -volname OpenWrt OpenWrt.sparseimage
hdiutil attach OpenWrt.sparseimage
```

logged `created: /Users/runner/work/openwrt/openwrt/OpenWrt.sparseimage`, then `/dev/disk8 GUID_partition_scheme`, `/dev/disk8s1 EFI`, `/dev/disk8s2 Apple_HFS /Volumes/OpenWrt`, and the rest of the job built inside `/Volumes/OpenWrt`. Sparse image creation and attachment work for the runner user with no `sudo`.

`GPSBabel/gpsbabel` run 33658832003 (02/09/2026), matrix job on `macos-26`, `Image: macos-26-arm64`:

```
hdiutil create GPSBabelFE.dmg -srcfolder GPSBabelFE.app -format UDZO -fs APFS -volname GPSBabelFE
hdiutil attach -noverify gui/GPSBabelFE.dmg
```

logged:

```
/dev/disk8    GUID_partition_scheme
/dev/disk8s1  Apple_APFS
/dev/disk9    EF57347C-0000-11AA-AA11-0030654...
/dev/disk9s1  41504653-0000-11AA-AA11-0030654...   /Volumes/GPSBabelFE
```

`disk8` is the image's partition map with an `Apple_APFS` partition, `disk9` is the synthesised APFS container that macOS creates for that partition (the type identifier beginning `EF57347C` is the APFS container type; `41504653`, ASCII "APFS", is the APFS volume type), and `disk9s1` is the volume mounted under `/Volumes`. The volume therefore lives in its own container with its own allocation, separate from the boot container, which is what the ENOSPC suite needs: filling `disk9s1` exhausts that 200 MiB container and never touches the runner's boot volume.

### Caveats for the suite

1. `actions/runner-images` issue #7522 "[macOS] hdiutil failures when creating DMGs" (closed): intermittent `hdiutil: create failed - Resource busy` on macOS 13 and later. The maintainer (vpolikarpov-akvelon) attributed it to `XProtectBehaviorService` briefly locking the newly created image and recommended a retry in the workflow. The suite should retry `hdiutil create` a few times on a non zero exit before failing.
2. `electron-userland/electron-builder` issue #9615 reports `hdiutil: attach failed - no mountable file systems` for HFS+ sparse images on a macOS 26.4 beta while APFS sparse images attached normally. The openwrt run shows HFS+ working on 26.5.2, but the suite should create the image with `-fs APFS`, which is also the default.
3. `actions/runner-images` issue #5426 confirms `hdiutil` is present on every macOS image (it is part of macOS).
4. The runner has 14 GB of storage (GitHub Docs); a 200 MiB sparse image is not a concern. Attach with `-nobrowse` and detach in `t.Cleanup` so a failed test does not leave a mounted volume for later tests in the same job.

The behaviour of APFS at its allocation floor inside a small container is a filesystem property, not a runner property, and this note does not attempt to establish it; the suite asserts it directly.

## Sub question 3: does a `gui/<uid>` domain exist on the runner, and what happens if not

### Apple's definitions

`launchctl(1)`: "`user/<uid>/[service-name]` Targets the user domain for the given UID or a service within that domain. A user domain may exist independently of a logged-in user." and "`login/<asid>/[service-name]` Targets a user-login domain or service within that domain. A user-login domain is created when the user logs in at the GUI and is identified by the audit session identifier associated with that login." `gui/<uid>` is "Another form of the login specifier" addressed by user rather than audit session id. The page also states that launchctl exits 0 on success and otherwise with an error code decodable by `launchctl error`.

Technical Note 2083, Table 1: the `Aqua` session type is the GUI launchd agent ("Has access to all GUI services; much like a login item"), `Background` is the per user context, `StandardIO` is for non GUI logins such as SSH; "If you do not specify the `LimitLoadToSessionType` property, `launchd` assumes a value of `Aqua`." The specification's plist sets `LimitLoadToSessionType` to `Aqua` (section 16), so it can only be bootstrapped into a GUI login domain.

### The hosted runner has a GUI login session

Established from the image build: `images/macos/scripts/build/configure-autologin.sh` ("Enabling automatic GUI login for the '$USERNAME' user..") writes `/Library/Preferences/com.apple.loginwindow autoLoginUser` and `autoLoginUserScreenLocked false`, and is run for the macOS 26 arm64 image by `images/macos/templates/macOS-26.arm64.anka.pkr.hcl` (provisioner block at lines 174 to 184, alongside `configure-tccdb-macos.sh`). `configure-machine.sh` closes Terminal during the build with `launchctl bootout gui/501/${term_service}`, which only works inside a live `gui/501` domain.

Established from a live run on the current `macos-latest` image: `basecamp/basecamp-cli` run 33803101317 (03/09/2026, `Image: macos-26-arm64`, `Version: 20260728.0273.1`) ran `/bin/launchctl managername` in a job step and logged `Aqua`; the same command over `ssh localhost` in the same job logged `Background`. A manager name of `Aqua` means the job step's own launchd context is the GUI login session of uid 501, so `gui/501` exists and is the domain a job step naturally addresses.

Established on the same runner architecture, earlier image: `lablup/all-smi` job 99893591496 (01/09/2026, `Image: macos-14-arm64`) gates its launchd tier on `launchctl print "gui/$(id -u)"` succeeding and otherwise emits a notice "no gui/$UID domain on this runner"; the log shows the tier running (`launchctl disable gui/501/com.lablup.all-smi`, `launchctl print gui/501/com.lablup.all-smi`) and no notice. `antoinedc/MantaUI` job 100047840201 (01/09/2026, `Image: macos-14-arm64`) runs an installer that uses `launchctl bootstrap gui/$(id -u)` and then `launchctl kickstart -k gui/$(id -u)/com.mantaui.server` and `launchctl print gui/$(id -u)/...`; the job completed successfully, and its own header comments state that it exists to prove "Both LaunchAgents load into the GUI domain and keep their process alive" on a GitHub hosted runner.

No run was found that bootstraps into `gui/$(id -u)` on the macOS 26 image specifically; the Aqua evidence on that image plus the bootstrap evidence on the macOS 14 image of the same autologin design is the basis for the verdict. The probe below closes that gap.

### What `launchctl` returns when the domain is absent

When no GUI session exists for the uid, `launchctl bootstrap gui/<uid> <plist>` fails with `Bootstrap failed: 125: Domain does not support specified action`. Evidence: `openclaw/openclaw` issue #46466 (macOS 26.3.1) quotes exactly that text, and the maintainers' triage records that the project classifies it as "requires a logged-in macOS GUI session for this user" and has a regression test on that string. `NousResearch/hermes-agent` issue #23387 (macOS 26.4.1) reports, from a session where `launchctl managername` was `Background`, `launchctl kickstart gui/502/...` exiting 125 with the same message and `launchctl bootstrap gui/502 ...` exiting 5 (`Input/output error`); the fix comment notes that bootstrapping into `user/<uid>` fails unless the plist declares `LimitLoadToSessionType` including `Background`. `launchctl print gui/<uid>` returns 125 in the same condition (`NousResearch/hermes-agent` issue #30586). The legacy `launchctl load` prints `Could not find domain for` (`buildkite/docs` issue #353). Exit code 5 also appears for malformed plists (`ProgramArguments` given as a string, Homebrew discussion #1372), so the suite should treat 125 as "no GUI domain" and 5 as "inspect the plist and `launchctl error 5`".

Because the specification's plist is `Aqua` only, `user/<uid>` is not a fallback for the product or the test without changing the plist; the correct behaviour when `gui/$UID` is absent is to skip the suite with a reason, and `launchctl print gui/$(id -u)` is the cheap pre-check (exit 0 means present).

## Sub question 4: TCC protected paths on the runner

### What the image build configures

`images/macos/scripts/build/configure-tccdb-macos.sh` inserts rows into the system TCC database (`/Library/Application Support/com.apple.TCC/TCC.db`) and the runner user's database. Rows for `kTCCServiceSystemPolicyAllFiles` (Full Disk Access) in the system database are granted to `/bin/bash`, `/opt/hca/start_hca.sh`, `/usr/libexec/sshd-keygen-wrapper`, `/usr/local/opt/runner/runprovisioner.sh`, `com.apple.Terminal`, `com.microsoft.wdav` and `com.microsoft.wdav.epsext`; in the user database to `/opt/hca/start_hca.sh` and `/usr/local/opt/runner/runprovisioner.sh`. The agent binary `/opt/hca/hosted-compute-agent` is granted Accessibility, Apple Events, Bluetooth, Microphone and Screen Capture but not `kTCCServiceSystemPolicyAllFiles`. A comment in the same script, written for the screen capture approval, says the approval is "keyed by the *responsible* process - on a hosted runner that is the agent, whatever tool the job actually runs". `System.Tests.ps1` asserts that approval for `/opt/hca/hosted-compute-agent`.

Two conclusions follow. First, TCC is active on the image (System Integrity Protection is enabled: issue #553 maintainer comment "the enabled SIP on Hosted macOS images"; the build spends effort pre-approving specific binaries because prompts would otherwise appear). Second, whether a Go test binary started from a `run:` step is credited to `/bin/bash` (which has Full Disk Access) or to the hosted compute agent (which does not) is not established from any source; the script comment points to the agent, which would make the "success interactively" half of the LaunchAgent assertion fail on CI with `EPERM`, while a bash attribution would make it pass. Apple does not document responsible process attribution for this topology.

### Whether `~/Library/Mail` exists and is protected

Not established. No image build script creates or touches `~/Library/Mail`, Mail is never launched during the build (the only reference is a `kTCCServiceUbiquity` row for `com.apple.mail`), and no public run log was found that lists it. If the directory is absent the test observes `ENOENT`, not `EPERM`, in both contexts. Apple publishes no first party list of the locations that Full Disk Access guards; that `~/Library/Mail` is one of them rests on community testing (clause verification required). Under launchd the transient agent's responsible process is the test binary itself, which has no TCC grant, so if the directory exists the `EPERM` half of the assertion is expected to hold on CI.

Runtime editing of the TCC databases is possible on hosted images: an `actions/runner-images` maintainer wrote in issue #9529 that "the current images of macOS 13 and 14 allow runtime TCC.db updating", and issue #14566 shows `sqlite3` inserts into the user database on macos-15 with only intermittent lock failures. This is noted for completeness; granting the test binary Full Disk Access to manufacture the "interactive success" would not test what the specification means, so it is not the recommended path.

## Verdicts

| Suite | Verdict | Basis |
|---|---|---|
| ENOSPC suite (section 20) | Runs on CI | `hdiutil create -type SPARSE` and `hdiutil attach` succeed for the `runner` user without `sudo` on the current `macos-26-arm64` image (openwrt run 33665818599, gpsbabel run 33658832003, both 02/09/2026); an APFS image attaches as its own synthesised container. Precautions: `-fs APFS`, `-nobrowse`, retry `create` on `Resource busy` (issue #7522), detach in cleanup, pin `macos-26`. Confidence high. |
| LaunchAgent suite (section 20) | Runs on CI with a named workaround | The job step runs in an Aqua session on the current image (basecamp-cli run 33803101317, 03/09/2026), so `launchctl bootstrap gui/$UID`, `print`, `print-disabled` and `kickstart` are available; hosted macos-14 arm64 runners bootstrap and kickstart agents in `gui/$(id -u)` in green runs. Workaround: pre-check `launchctl print gui/$(id -u)` and skip with a reason on failure; make the `~/Library/Mail` assertion conditional (skip with a reason when the directory is absent, and record rather than assert the interactive result until the probe has shown which process TCC credits). Confidence high for the launchd half, low for the TCC half until the probe runs. |

Milestone v0.1.0 should run the probe once and attach its output to issue #5 or the map; milestone v0.3.0 then decides gating from established results rather than from this inference.

## Probe workflow (do not add to the repository without a ticket)

One manual job that prints every fact this note could not establish, including the two `hdiutil` operations and a transient agent that reads `~/Library/Mail` under launchd. Every step uses `set +e` and prints exit codes; the job never fails on a probe result.

```yaml
name: runner-probe

on:
  workflow_dispatch:

permissions:
  contents: read

jobs:
  probe:
    runs-on: macos-26
    timeout-minutes: 10
    steps:
      - name: Image, user and privileges
        run: |
          set +e
          sw_vers
          uname -m
          id
          echo "admin membership: $(dsmemberutil checkmembership -U "$(id -un)" -G admin)"
          sudo -n true; echo "sudo -n true exit=$?"
          csrutil status

      - name: launchd domains
        run: |
          set +e
          echo "managername: $(launchctl managername)"
          launchctl print "gui/$(id -u)" >/dev/null 2>&1; echo "print gui/$(id -u) exit=$?"
          launchctl print "user/$(id -u)" >/dev/null 2>&1; echo "print user/$(id -u) exit=$?"
          launchctl print-disabled "gui/$(id -u)" >/dev/null 2>&1; echo "print-disabled gui/$(id -u) exit=$?"

      - name: TCC protected path, interactive
        run: |
          set +e
          ls -ld "$HOME/Library/Mail"; echo "ls -ld Mail exit=$?"
          ls "$HOME/Library/Mail" >/dev/null; echo "ls Mail exit=$?"
          ls -ld "$HOME/Library/Messages" "$HOME/Library/Safari" 2>&1
          sqlite3 "$HOME/Library/Application Support/com.apple.TCC/TCC.db" \
            "select service, client from access where service like '%AllFiles%';" 2>&1
          echo "user TCC query exit=$?"

      - name: TCC protected path, under a transient LaunchAgent
        run: |
          set +e
          label="com.geraldcsoftware.dusk.probe"
          log="$RUNNER_TEMP/agent.log"
          plist="$RUNNER_TEMP/$label.plist"
          cat > "$plist" <<EOF
          <?xml version="1.0" encoding="UTF-8"?>
          <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
          <plist version="1.0"><dict>
            <key>Label</key><string>$label</string>
            <key>ProgramArguments</key><array>
              <string>/bin/sh</string><string>-c</string>
              <string>echo "managername: \$(launchctl managername)"; ls -ld "\$HOME/Library/Mail"; echo "ls -ld exit=\$?"; ls "\$HOME/Library/Mail"; echo "ls exit=\$?"</string>
            </array>
            <key>LimitLoadToSessionType</key><string>Aqua</string>
            <key>RunAtLoad</key><true/>
            <key>StandardOutPath</key><string>$log</string>
            <key>StandardErrorPath</key><string>$log</string>
          </dict></plist>
          EOF
          plutil -lint "$plist"
          launchctl bootout "gui/$(id -u)/$label" 2>/dev/null
          launchctl bootstrap "gui/$(id -u)" "$plist"; echo "bootstrap gui exit=$?"
          sleep 3
          launchctl print "gui/$(id -u)/$label" | head -40; echo "print service exit=$?"
          echo "--- agent log ---"; cat "$log" 2>/dev/null
          launchctl bootout "gui/$(id -u)/$label"; echo "bootout exit=$?"

      - name: hdiutil sparse APFS image without sudo
        run: |
          set +e
          img="$RUNNER_TEMP/probe.sparseimage"
          for attempt in 1 2 3; do
            hdiutil create -size 64m -type SPARSE -fs APFS -volname dusk-probe "$img" && break
            echo "create attempt $attempt failed"; sleep 2
          done
          hdiutil attach -nobrowse -owners off "$img" | tee "$RUNNER_TEMP/attach.txt"; echo "attach exit=$?"
          vol=$(awk '/\/Volumes\//{print $NF}' "$RUNNER_TEMP/attach.txt")
          dev=$(awk 'NR==1{print $1}' "$RUNNER_TEMP/attach.txt")
          diskutil info "$vol" | egrep 'Device Node|Container|File System|Volume Free Space|Volume Total Space'
          diskutil apfs list "$dev" 2>&1 | head -30
          dd if=/dev/zero of="$vol/fill" bs=1m 2>&1; echo "dd exit=$? (ENOSPC expected)"
          df -k "$vol"
          rm -f "$vol/fill"; echo "unlink after floor exit=$?"
          hdiutil detach "$vol"; echo "detach exit=$?"
```

## Sources

- `actions/runner-images` readme at `main`, commit `148c0a4a` (20/08/2026), "Available Images" table and the "-latest" migration note: https://github.com/actions/runner-images/blob/main/README.md
- `actions/runner-images` macOS 26 arm64 readme: https://github.com/actions/runner-images/blob/main/images/macos/macos-26-arm64-Readme.md
- `actions/runner-images` image build scripts: `images/macos/scripts/build/configure-autologin.sh`, `configure-machine.sh`, `configure-tccdb-macos.sh`, `configure-system.sh`; template `images/macos/templates/macOS-26.arm64.anka.pkr.hcl`; test `images/macos/scripts/tests/System.Tests.ps1`
- `actions/runner-images` issues: #7522 (hdiutil `Resource busy`, XProtectBehaviorService), #5426 (hdiutil present on all images), #553 (SIP enabled on hosted images), #9529 (runtime TCC.db updating allowed), #14566 (TCC.db inserts on macos-15), #113 (Full Disk Access for bash, 2020), #13518 (macOS 14 deprecation)
- GitHub Docs, GitHub-hosted runners: https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- `launchctl(1)` manual page: https://keith.github.io/xcode-man-pages/launchctl.1.html
- `hdiutil(1)` manual page: https://keith.github.io/xcode-man-pages/hdiutil.1.html
- Apple Technical Note 2083, Daemons and Agents: https://developer.apple.com/library/archive/technotes/tn2083/_index.html
- Apple, Creating Launch Daemons and Agents: https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html
- Run logs: `basecamp/basecamp-cli` run 33803101317 (workflow `.github/workflows/headless-probe-composition.yml`, `macos-latest`); `openwrt/openwrt` run 33665818599 (reusable workflow `openwrt/actions-shared-workflows/.github/workflows/tools.yml`, `macos-latest`); `GPSBabel/gpsbabel` run 33658832003 (`.github/workflows/macos.yml`, `macos-26`); `lablup/all-smi` job 99893591496 (`.github/workflows/ci.yml`, `macos-14`); `antoinedc/MantaUI` job 100047840201 (`.github/workflows/macos-install-smoke.yml`, `macos-14`); `radareorg/iaito` run 33815771177 (`macos-latest`)
- Failure mode reports: `openclaw/openclaw` issue #46466; `NousResearch/hermes-agent` issues #23387 and #30586; `buildkite/docs` issue #353; Homebrew discussion #1372; `electron-userland/electron-builder` issue #9615
