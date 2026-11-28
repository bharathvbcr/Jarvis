# Quickstart: 002 - Discovery, Deterministic Replay, Independent Verification, and Pinned Upstreams

Run from the repository root `/Users/bharath/Code/devtools/Jarvis`.

1. Run the Go suites (no desktop and no provider are needed):
   ```bash
   CGO_ENABLED=0 GOMAXPROCS=4 go test -count=1 -p 2 ./internal/... ./cmd/...
   ```
2. Fetch the pinned upstreams and check they match the lock:
   ```bash
   go run ./cmd/dev bootstrap
   go run ./cmd/dev check-pins
   ```
3. Build and replay the example capability against the synthetic bank (needs a desktop session and, on macOS, accessibility and screen-capture permission):
   ```bash
   go run ./cmd/dev build
   ./build/jarvis doctor
   ./build/jarvis replay --events --capability examples/balance.json --tenant north
   ```
4. Verify a recorded bundle against an acceptance contract:
   ```bash
   ./build/jarvis verify --contract scenarios/balance-m1001.contract.json --bundle <bundle>
   ```
5. Validate the specifications:
   ```bash
   go run ./cmd/dev check-specs
   ```
