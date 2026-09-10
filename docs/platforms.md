# Native platform qualification

Compilation, permissions, native execution, and a complete tenant campaign are separate results. A platform qualifies only after a real desktop campaign completes every requested read-only run for both tenant layouts, with independent UI verification and an unchanged synthetic-bank state oracle. Twenty repeated successes do not establish production reliability; the report includes an exact one-sided binomial lower bound and explicitly declines to assume independent trials.

## Current evidence

The pinned macOS rebuild has a separate `evidence/macos-pinned-40.json` campaign:
23 passed, 17 suspended after native input-counter changes, all 40 saved-state
oracles unchanged. It does not pass the clean stability gate. The current denial
campaign reached denial on South; North suspended before approval. UTM now crashes
before guest startup, so Linux refresh attempted zero runs; see
`evidence/linux-refresh-blocked.json`. The historical successful campaigns below
remain bound to their original binary hashes.

| Platform | Compilation | Native desktop execution | Qualification |
| --- | --- | --- | --- |
| macOS ARM64 | Local build and 40 native-workspace tests pass | Scoped ScreenCaptureKit/AX lookup and visual resolution with unapproved-click refusal verified; raw pixels stayed in memory | Retained readiness campaign: 20/20 per tenant; zero first-attempt successes; exact tested binary hashes in report |
| Linux ARM64 X11 | ARM cross-check and real guest native/bank/verifier builds pass | Real AT-SPI observation, X11 capture and fenced XTest editable-field input in Ubuntu/Xfce | Retained campaign: 20/20 per tenant; one first-attempt success; denial/overlay/missing/duplicate cases each 2/2 expected safety behavior |
| Windows ARM64 | Rust `cargo check` and Jarvis Go ARM64 cross-build pass | Not executed | User explicitly deferred Windows qualification |

The macOS-hosted Rust check collected 15 core tests, one portable Linux-adapter test, three macOS adapter tests, and 21 broker/process tests (including one subprocess fixture). Linux-native tests are reported separately. A successful cross-check does not prove linking, desktop accessibility, capture, permissions, graphics, or native event delivery on the destination OS. Process supervision FFI lives in `Manvi/native/crates/manvi-desktop/src/platform/`; the broker module forbids unsafe Rust.

The macOS hardware admission guard compares fifteen native HID event counters around observations and immediately before input, then suspends for reconciliation on a mismatch. Counters remain private to the broker, and epoch transfer is explicit. Tests cover comparison/wrap refusal, privacy and pause/resume fencing, and use the real CoreGraphics API to verify a private event source differs from the HID source without posting any input. Actual hardware-versus-injected classification is not qualified. This guard is not continuous monitoring, does not classify other programs' synthetic input, and has no Windows/X11 listener implementation. Those requirements remain explicit runtime/implementation gates; ordinary semantic replay still uses native state, recipient and foreground checks.

The visual native probe created a protected-region-checked 120×30 PNG anchor from the Search label, resolved it uniquely inside the attached 1760×1464 bank frame, and received `approval_required/not_sent` for a visual read click without a grant. The native public target omitted the private frame digest, and the bank-state oracle remained unchanged. This proves observation, matching, privacy-boundary shape and approval refusal; no human grant or live visual click was exercised. Exact matching deliberately refuses changing pixels, hover effects, scale changes and ambiguity. See `evidence/examples/macos-visual-no-grant/` for the report, sanitized image and provenance.

## One canonical campaign runner

From the Jarvis project root, after building its existing Manvi and DevCouncil sources:

```text
go run ./cmd/dev build --manvi /path/to/Manvi --devcouncil /path/to/DevCouncil
go run ./cmd/qualify --n 20 --tenants north,south --scenario balance --out evidence/platform-fresh-40.json
```

The default starts a fresh synthetic bank for every run. The runner refuses an existing report path, preserves completed attempts on interruption, bounds each run and its cancellation, and records native status/permission failures as blocked. A missing oracle is unexamined, not a passing oracle. Reports include requested and attempted counts, per-tenant totals, native observation retries, and separate `matrix_complete` and `qualified` fields.

For twenty read-only replays against one already running synthetic bank:

```text
go run ./cmd/qualify --pid 12345 --tenants north --oracle-state /absolute/path/to/synthetic-state.json --n 20 --scenario balance --out evidence/platform-north-same-process-20.json
```

Replace the PID and state path with those of that exact synthetic bank. This mode requires one explicit tenant and records a partial matrix; it cannot qualify the whole platform. It is useful for distinguishing new-window/Stage Manager transitions from steady-process behavior. Both modes allow explicit `--broker`, `--verifier`, `--bank`, and `--run-timeout` paths/settings.

Fault campaigns use `--scenario denial`, `overlay`, `delay`, `missing-control`, `duplicate-control`, `commit-noop`, or `crash-after-commit`. They never approve a business mutation. `fault_reached` requires observed evidence; a requested fault flag is not evidence it ran. In particular, denial before Confirm does not exercise commit-noop or crash-after-commit. Those post-commit cases require a separate human-approved test and reconciliation of the independent state oracle.

## Reproducible Linux ARM64 lab on Apple Silicon

The Go preparation tool uses official UTM configuration and vendor command-line tools, with no first-party shell, Python, or Swift implementation:

```text
go run ./tests/native-platform prepare-linux --out "tests/native-platform/local/Jarvis Linux X11.utm" --register
utmctl list
utmctl start VM-UUID
utmctl status VM-UUID
utmctl ip-address VM-UUID
utmctl exec VM-UUID --cmd /usr/bin/cloud-init status --long
utmctl exec VM-UUID --cmd /usr/bin/cat /run/cloud-init/result.json
utmctl exec VM-UUID --cmd /usr/bin/loginctl show-session c1 -p Type -p State -p Display -p Name
```

Preparation refuses an existing bundle and preserves a named `.preparing` directory if creation fails. It verifies SHA-256 before using the root image, kernel, or initrd; extraction accepts only the expected bounded raw ext4 image. The root disk has 64 GiB sparse capacity, initially about 2 GiB allocated. The guest receives four vCPUs and 6 GiB RAM, Xfce/LightDM on X11, AT-SPI, and Mesa software rendering. Network uses QEMU user-mode NAT without forwarded ports. Clipboard sharing, host directory sharing, USB sharing, and sound are disabled. The synthetic `jarvis` guest account uses LightDM autologin, a locked password and guest-local sudo; it receives no host credentials or SSH key.

The Ubuntu image is pinned to [Canonical's 20260826 Noble ARM64 artifacts](https://cloud-images.ubuntu.com/noble/20260826/). Its raw filesystem format and separate kernel/initrd are documented in [Ubuntu's artifact reference](https://ubuntu.com/docs/public-images/public-images-reference/artifacts/). The SHA-256 values are:

| Asset | SHA-256 |
| --- | --- |
| `noble-server-cloudimg-arm64.tar.gz` | `6a0c8d75491988f7a51d443be6a8b36455b53684fa87245d1478d5b1643bbbbd` |
| `unpacked/noble-server-cloudimg-arm64-vmlinuz-generic` | `a6c429cb79db29b987d138d1e8b2a6c9f0bbad28023145e2db7fe95505e96c9d` |
| `unpacked/noble-server-cloudimg-arm64-initrd-generic` | `f6082d78117fbfc35ccf2aebc7d48b878cbcfe4f97bfa7f40acab48b46569b18` |

These were checked against Canonical's HTTPS checksum files. GPG verification of the checksum signature has not been performed. The pinned image can eventually leave Canonical's retention window; the tool must fail rather than substitute an unverified image.

Direct kernel boot explicitly selects `ds=nocloud`; the `CIDATA` ISO attaches as a read-only VirtIO disk, making it visible to early cloud-init discovery. The seed includes a MAC-matched network configuration. The serial console is primary (`ttyAMA0,115200`) so boot failures are inspectable. Package sources use Canonical HTTPS, omit recommended packages, and bound download retries. Despite an explicit host RTC and a boot mask for `systemd-timesyncd`, the initial guest clock was observed in July while the host was in September. The successful seed sets the preparation timestamp through cloud-init `bootcmd` before HTTPS package retrieval; certificate verification stays enabled. This timestamp is not a lasting clock synchronization service: after later boots, verify UTC against the host and correct it before package retrieval or evidence collection. Host clock accuracy remains a prerequisite. These settings follow [NoCloud's documented discovery and seed format](https://github.com/canonical/cloud-init/blob/main/doc/rtd/reference/datasources/nocloud.rst), [APT configuration](https://github.com/canonical/cloud-init/blob/main/doc/module-docs/cc_apt_configure/example1.yaml), and [systemd's documented per-boot mask](https://github.com/systemd/systemd/blob/v255/man/systemd-debug-generator.xml).

UTM 4.7.5's actual configuration encoding and QEMU argument generation were inspected in [its source](https://github.com/utmapp/UTM/blob/v4.7.5/Configuration/UTMQemuConfiguration%2BArguments.swift). Administration uses [the documented UTM CLI](https://docs.getutm.app/scripting/scripting/). `utmctl ip-address` can return exit status zero while printing that the guest agent is unavailable; inspect its response. In 4.7.5, `utmctl attach` reports the PTTY path but says the attach operation is not implemented. Connect a terminal to that specific guest PTTY to inspect serial output. UTM's saved guest screenshot can show the last boot screen; it is not a current native bank observation.

After provisioning, build the Rust workspaces and Go host inside the guest, then run the campaign inside the real logged-in `jarvis` X11 session. Confirm `XDG_SESSION_TYPE=x11`, `DISPLAY`, the user's DBus session, and an enabled AT-SPI bus. `LIBGL_ALWAYS_SOFTWARE=1` must be in the desktop environment before launching egui. Root or an SSH session without the desktop's bus and Xauthority is not a valid qualification environment. Record `uname`, compiler versions, `glxinfo -B`, cloud-init status and broker probe alongside the campaign.

The provisioned local guest is UUID `41990217-EBE5-483F-9821-056DF0FD571A`, in the ignored `tests/native-platform/local/Jarvis Linux X11 Network.utm` bundle. Its desktop user is UID 1000, with `DISPLAY=:0` and `XAUTHORITY=/home/jarvis/.Xauthority`. Transfer files through the guest agent using `utmctl file push VM-UUID /guest/path < /host/file` and `utmctl file pull VM-UUID /guest/path > /host/file`. An exit-zero guest command with empty output is insufficient evidence that a graphics or accessibility check passed.

## Windows ARM64 execution, deferred

The user will run Windows qualification later. No Windows media was acquired and no license terms were accepted. Use the user's licensed ARM64 Windows installation, an interactive standard-user desktop, the built asInvoker/PerMonitorV2 native broker and software-compatible egui rendering. Build on Windows ARM64 with its supported Rust MSVC toolchain and Go, then use the same `cmd/dev` and `cmd/qualify` commands. VM graphics, UI Automation, HWND capture, foreground admission, UIPI and mixed-DPI behavior remain execution gates.

Cross-check commands from the Manvi native workspace:

```text
cargo check --locked -p manvi-desktop --target aarch64-pc-windows-msvc
cargo check --locked -p manvi-desktop --target aarch64-unknown-linux-gnu
```

From the Jarvis root, `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/jarvis` also cross-builds the host. Manvi's shared `internal/safefile` uses kernel no-follow opens, rejects Windows reparse handles before content I/O, captures file identity before approval, and obtains hardlink counts from the open handle. macOS race tests pass; Windows test executables cross-compile, with Windows runtime validation deferred alongside the desktop campaign.

## Additional platform cases

Beyond the repeated bank campaign, retain explicit cases for minimized/occluded windows, geometry changes, 1×/2× and mixed-DPI displays, duplicate controls, stale observation handles, accessibility permission revocation, a blocked helper during pause/EOF, and human/automation ownership transfer. Linux X11 capture must reject occlusion; Windows PrintWindow must demonstrate useful pixels for the target software renderer. A native API returning success is only a dispatch receipt: set-value and press must be followed by independent observed state. Never retry an input with `unknown` delivery before reconciliation.
