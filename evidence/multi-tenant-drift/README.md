# One artifact across two tenants and two vendor variants

These are live `jarvis drift` reports, each taken against a running bank window on
macOS. They answer the reuse half of §3.7: how the same recorded capability serves
tenants running the same underlying product configured differently, and how drift is
noticed instead of silently absorbed.

| Report | Overlay applied | Compiled digest | `search` resolved at |
| --- | --- | --- | --- |
| [north-base](north-base.json) | none | `12396b8c6cd5` | rung 0 |
| [south-default-variant](south-default-variant.json) | `overlays/south-balance.json` | `f43d6fe841e1` | **rung 1** |
| [south-renamed-controls](south-renamed-controls.json) | `overlays/south-balance.json` | `f43d6fe841e1` | rung 0 |

`bank.balance` revision 1 is recorded once. North runs it unmodified. South declares
one overlay that replaces the `search` ladder with `Find member` at rung 0 and keeps
`Search` as rung 1, because South is the variant that renamed that control.

The two South rows are the same tenant, the same artifact and the same overlay against
two builds of the product. On the renamed-controls variant the specialization matches
at rung 0. On the default variant it does not, and the run still succeeds — at rung 1,
the fallback. Nothing is silently absorbed: the report names the rung that matched, so
"South is running a build my overlay no longer describes" is an observable fact rather
than a slow failure.

Three properties make this safe to do at scale rather than re-recording per tenant:

- **The artifact names no surface.** `workflow.Capability` has no window, process or
  screen field. The concrete surface comes from the tenant profile at compile time and
  from `Session` at run time, so one recording is not bound to one installation.
- **Specialization is bounded and identified.** An overlay may only replace target
  ladders and narrow limits, never widen a budget or introduce a target key; every rung
  it introduces must already be in the reviewed policy allowlist. A specialized
  compile has its own digest — `sha256(base ‖ overlay)` — so North and South are
  distinguishable artifacts in evidence rather than the same id behaving differently.
- **Overlays are selected by the capability they name.** A profile carries a set, and a
  capability no overlay names compiles from the reviewed base.

## Limits

These are read-only lookups on a synthetic bank with two layouts, not hundreds of
tenants. Nothing here measures how a ladder behaves across a real vendor's version
skew, and rung 1 succeeding is evidence that the fallback works, not that the overlay
is still correct — deciding when a fallback should become an alert is not implemented.
