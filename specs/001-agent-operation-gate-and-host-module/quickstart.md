# Quickstart: 001 - Agent Operation Gate, Host Module, and Background Jobs

Run from the repository root `/Users/bharath/Code/devtools/Jarvis`. The Go workspace needs the pinned Manvi checkout (`go run ./cmd/dev bootstrap`); `go.work` is already present in this checkout.

1. Run the gate and host tests:
   ```bash
   CGO_ENABLED=0 GOMAXPROCS=4 go test -count=1 -p 2 ./internal/app/ ./cmd/jarvis/
   ```
2. List the served operations with their classes (needs a built host, `go run ./cmd/dev build`):
   ```bash
   ./build/jarvis ops
   ```
3. Call an `agent` operation and observe a refusal for a human-only one:
   ```bash
   ./build/jarvis call jarvis.catalog.list
   ./build/jarvis call jarvis.run.act      # exit 3, E_AGENT_REFUSED
   ```
4. Validate the specifications:
   ```bash
   go run ./cmd/dev check-specs
   ```
