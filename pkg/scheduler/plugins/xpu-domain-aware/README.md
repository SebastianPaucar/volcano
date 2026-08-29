# `xpu-domain-aware` — PoC

Standalone proof-of-concept for [volcano-sh/volcano#5751](https://github.com/volcano-sh/volcano/issues/5751) ("Generic xPU Topology-Aware Scheduling"), built to validate Jesse Stutler's stated direction against real Volcano scheduler code:

- *"Add domain related info before HyperNode/Node selection"*
- *"Build a generic, device-agnostic solution... the final XPU topology-aware scheduling mechanism must be universal"*

## What it does

Registers on `AddHyperNodeGradientForSubJobFn` — the same extension point `network-topology-aware.go` uses — and filters candidate HyperNodes down to ones containing a device domain with enough free devices for the SubJob's request. This runs inside `allocateForSubJob`, **before** `PredicateNodes` or any device `Allocate()` call executes.

Topology comes from a minimal, vendor-neutral interface:

```go
type TopologyProvider interface {
    DomainsForNode(nodeName string) ([]TopologyDomain, bool)
    Refresh() error
}
```

A `MockTopologyProvider` backs it for this PoC, consistent with the project's own "mock topology data and KWOK" allowance. The plugin has no HAMi/mindcluster/vgpu-specific code anywhere — swapping the mock for a real annotation/CRD/DRA-based provider wouldn't require touching the plugin.

## Files

| File | Purpose |
|---|---|
| `topology.go` | `TopologyDomain`, `TopologyProvider` interface, `MockTopologyProvider` |
| `xpu_domain_aware.go` | The gradient-filtering plugin (`PluginName = "xpu-domain-aware"`) |
| `xpu_domain_aware_test.go` | Two tests against real `allocate`/`predicates`/`gang` plugins |

## Tests

**`TestXPUDomainAware_FiltersHyperNodeWithSplitDomains`**
Two HyperNodes both report 8 idle devices in aggregate. One has them split 4/4 across two domains (no single domain can satisfy an 8-device request); the other has one domain covering all 8. Without domain awareness these look identical to the scheduler. With the plugin, only the genuinely satisfying HyperNode survives the gradient and receives the bind — confirmed via full trace: the split-domain HyperNode is rejected at the gradient stage and never reaches `PredicateNodes`.

**`TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation`**
No HyperNode anywhere has a domain that satisfies the request. The SubJob finds no gradient solution and is not bound — instead of the current behavior (bind blindly, discover the domain mismatch at device-allocation time, after the Node/HyperNode are already committed).

**`TestXPUDomainAware_MultipleSatisfyingHyperNodesBindsToOne`**
Three HyperNodes: one fails the domain filter (split 4/4), two pass (each has one domain covering all 8). Verifies a gradient tier with multiple survivors flows correctly through `selectBestHyperNodeForSubJob`'s scoring/selection — the earlier development bug (survivors not composing with the rest of the allocate pipeline) is exactly what this guards against.
 
**`TestXPUDomainAware_MissingDomainDataTreatedAsUnsatisfying`**
Reproduces the mindcluster/vgpu gap from the issue thread: one Node has no entry in the `TopologyProvider` at all (not an empty domain list — no domain concept whatsoever, matching vendor plugins with nowhere to report per-device domain membership). Confirms missing domain data is treated as unsatisfying, never silently let through as "no constraint."

## Run it

```bash
go build ./pkg/scheduler/plugins/xpu-domain-aware/...
go vet ./pkg/scheduler/plugins/xpu-domain-aware/...
go test ./pkg/scheduler/plugins/xpu-domain-aware/... -v -run TestXPUDomainAware

=== RUN   TestXPUDomainAware_FiltersHyperNodeWithSplitDomains
--- PASS: TestXPUDomainAware_FiltersHyperNodeWithSplitDomains (0.37s)
=== RUN   TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation
--- PASS: TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation (0.35s)
=== RUN   TestXPUDomainAware_MultipleSatisfyingHyperNodesBindsToOne
--- PASS: TestXPUDomainAware_MultipleSatisfyingHyperNodesBindsToOne (0.38s)
=== RUN   TestXPUDomainAware_MissingDomainDataTreatedAsUnsatisfying
--- PASS: TestXPUDomainAware_MissingDomainDataTreatedAsUnsatisfying (0.35s)
PASS
ok  	volcano.sh/volcano/pkg/scheduler/plugins/xpu-domain-aware	1.513s
```

## What this proves

1. The hook point works. `HyperNodeGradientForSubJobFn` can carry domain data before HyperNode/Node selection with no new scheduler machinery — it composes with `network-topology-aware` and, per PR #5349's pattern, would compose with `group-topology-affinity` the same way.
2. The interface is genuinely vendor-neutral, not just in principle.
3. It changes real scheduling outcomes, verified against actual Volcano plugin code — not a standalone toy harness.

## Integration detail worth flagging

`HyperNodeGradientForSubJobFn` is called with the **search root** (`ClusterTopHyperNode`), not individual candidates. A gradient plugin has to expand `ssn.HyperNodes` / `ssn.RealNodesList` itself and filter the expansion — mirroring what `network-topology-aware`'s BFS-based `hyperNodeGradientFn` already does internally. This wasn't obvious from the design docs and only surfaced while wiring this PoC against the real framework code.

## Explicitly out of scope

This PoC proves the mechanism, not the full project. Left out:

- Concrete device ID selection (`AllocateFunc`/`DeAllocateFunc` wiring)
- Reservation and gang-level rollback
- Cross-node fabric domains (Case 3, e.g. GB200 NVL72)
- Real Device Plugin / DRA `ResourceSlice` ingestion (mock provider only)

These are the actual project scope (Expected Outcomes 1–3 in the issue), not something a PoC of this size should attempt.