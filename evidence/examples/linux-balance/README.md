# Genuine Linux X11 balance run

These 20 evidence files were copied byte-for-byte from the isolated Linux guest's
completed North Cooperative run `09a8bb6c81691e2e5b8c16d6e275e61d`. The bundle,
contract, screenshots, accessibility trees, journal, and verifier result were not
rewritten. This README is the only added file.

The run is North trial 1 in [the 40-trial report](../../linux-x11-final-40.json).
It completed in 4,603 ms, used one bounded capture retry and no input retries,
passed all three independent evidence criteria, and left seeded member balances
and subaccounts unchanged. All 40 campaign runs passed. There were 39 capture
retries across the campaign, zero model requests, and zero model cost; this is
not a claim that every initial capture succeeded or that 40 samples establish
95% population reliability.

Environment: Ubuntu ARM64, XFCE X11, AT-SPI and XTest, Mesa software rendering,
Virtual-1 at 1440x900. The synthetic AccessKit application required
`org.a11y.Status.ScreenReaderEnabled=true` in its session; `IsEnabled=true` alone
did not register its accessibility tree. The original 1280x800 display clipped
the scaled bank into the bottom panel. The adapter retained its strict occlusion
check and the runner retried only explicit not-sent capture failures.

Input used the actual AT-SPI tree, a scoped X11 screenshot, the existing X11
keyboard mapping for the editable member field (AccessKit lacked EditableText),
and AT-SPI Search activation. The test did not use a browser, a mock desktop,
an LLM, or an approval grant. Member-ID pixels and values in public evidence are
redacted. External-input monitoring on Linux remains unavailable.

The report pins the exact bank, broker, qualifier, and verifier hashes. Its
binary/source provenance is limited to those artifacts: the guest did not have
Git source metadata. The later optional macOS HID guard and the corresponding
Linux `input_stamp: None` source initializer were not part of these frozen
campaign binaries. Earlier failed reports remain in the evidence inventory.
