# Pinned macOS balance replay

Run `f4ac19f79c6e258edffd03470d4c821f` is the first North run in
`../../macos-pinned-40.json`. It used the exact executables recorded there and in
`../../build-verification.json`, with Manvi revision `4818dc2081a53240eb8f4bde48026ad8d704e351`.

The real bank returned 125000 minor USD, independent evidence verification passed
all three criteria, and the separate saved-state oracle remained unchanged.
Offline replay reconstructs completion. The copied artifact bytes are unchanged;
the final masked capture was visually inspected. No provider call or business
approval occurred. This one success does not qualify the entire campaign, which
also contains 17 input-counter suspensions.
