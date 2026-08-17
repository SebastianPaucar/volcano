# Case 2 PoC — fragmented device domains are invisible until after Node/HyperNode commit

**Status: working, verified against real scheduler code, 8/8 runs consistent with the finding.**

## What this proves

Volcano's `allocate` action selects a HyperNode, then a Node, using only *aggregate* free-resource counts. `Devices.FilterNode` — the hook that could reject a node whose free devices are fragmented across physical domains — is wired into `AddSimulatePredicateFn` (preemption dry-run only), **never into the real `Predicate()` path** that gates actual node selection. Real device allocation (`Devices.Allocate`) only runs *after* Node/HyperNode are already committed, inside the `AllocateFunc` event handler in `predicates.go`. If it fails there, `allocateResourcesForTask` logs the error and moves on — there is no backtracking to try a different Node within the same HyperNode.

This PoC reproduces that failure mode end-to-end against real, shipped Volcano scheduler code — the actual `allocate` action, the actual `predicates` and `gang` plugins, the actual `Statement`/rollback machinery — with only the device backend mocked (the same seam real vendor plugins like ascend/nvidia plug into).

## Files

- `mock_topology_device.go` -> `pkg/scheduler/api/devices/mocktopology/mock_topology_device.go`
- `case2_fragmentation_poc_test.go` -> `pkg/scheduler/actions/allocate/case2_fragmentation_poc_test.go`

## How to run

From `volcano`:

```bash
go build ./pkg/scheduler/api/devices/mocktopology/...
go vet ./pkg/scheduler/actions/allocate/...
go test ./pkg/scheduler/actions/allocate/ -run TestCase2_FragmentedDomainNotVisibleAtSelectionTime -v -count=8
```

## Scenario

- `frag-node`: two device domains, 6 free + 2 free (8 aggregate free)
- `healthy-node`: one device domain, 8 free (8 aggregate free)
- One gang task requesting 8 devices as a single contiguous domain

Both nodes report identical aggregate free counts, so nothing in the gradient/predicate/score path can prefer one over the other.

## Results (8 real runs against the compiled scheduler)

| Outcome | Runs | What happened |
|---|---|---|
| `frag-node` picked | 3/8 | `Allocate()` called once, fails cleanly (no domain fits 8), `healthy-node` never touched -- **this is the core finding, reproduced cleanly** |
| `healthy-node` picked, succeeds | 1/8 | `Allocate()` called once, succeeds |
| `healthy-node` picked, then fails anyway | 4/8 | `Allocate()` called **twice** on the same node/pod -- see below |

### Finding 1 (primary -- this is what #5751 needs)

> Confirmed empirically against real Volcano scheduler code: when a Node's aggregate free device count satisfies a gang task's request but the request cannot be satisfied by any single physical domain, `HasDeviceRequest`/`FilterNode` are never consulted on the real allocate path. Only `AllocateFunc`'s post-selection `Devices.Allocate()` call discovers the fragmentation -- and it does so *after* HyperNode and Node have both already been committed for that attempt. There is no retry against a different Node within the same cycle; the task is left unbound and the podgroup stays pending.

Reproduced cleanly in runs 2, 6, 8 of 8: `frag-node` selected, one
`Allocate()` call, clean failure, `healthy-node` untouched, no fallback.

### Finding 2 (secondary -- separate, real, worth its own issue)

In 4/8 runs, `healthy-node` was picked, its single `Allocate()` call succeeded (consumed all 8 devices), and then a **second** `Allocate()` call fired for the *same* pod on the *same* node -- this time correctly failing since 0 devices remained. The accompanying log line (`predicates, remove pod c1/p1 from node [healthy-node] error: no corresponding pod p1 in pods of node healthy-node`) shows the rollback path trying to remove a pod that was never actually added to the node's pod list. This points to `Statement`'s rollback (`statement.go:304`) re-invoking `AllocateFunc` during unwind without the corresponding node-level pod bookkeeping having been added first -- a partial/asymmetric rollback. This is orthogonal to Case 2 and should **not** be folded into the #5751 argument; it's evidence for a separate bug in `statement.go`'s discard/rollback sequencing.

## Known caveats (read before citing)

- **Non-determinism is expected and is itself evidence.** Both nodes report identical aggregate free counts, so which one wins the Node-selection tie-break varies by run -- that's the point: the scheduler has no signal to prefer the healthy node. For a deterministic 100%-repro instead of a 3-outcome split, bias `frag-node`'s aggregate CPU/memory slightly higher so nodeorder/binpack scoring always picks it.
- **Bind status isn't directly inspected.** `TestCommonStruct.binder` is unexported and only reachable from inside `pkg/scheduler/uthelper`. The PoC relies entirely on the device call counters/log instead, which are set synchronously inside `Allocate()`/`FilterNode()` and are sufficient evidence on their own. If you want the actual bind outcome too, add:
  ```go
  func (test *TestCommonStruct) Binds() map[string]string {
      return test.binder.(*util.FakeBinder).Binds()
  }
  ```
  to `helper.go` (pure read, mirrors what `CheckBind` already does internally).

## Suggested next step for the real fix

- Extend `hyperNodeGradientFn`'s `isEligibleHyperNode` pre-filter to accept an optional domain-aware predicate (not just aggregate `idle`/`futureIdle`), so a HyperNode can be excluded before Node scoring if no candidate Node inside it has a domain that fits.
- Wire `Devices.FilterNode` into `predicates.go`'s real `Filter()` loop (currently only `AddSimulatePredicateFn` calls it), so Node-level filtering also rejects fragmented nodes pre-commit.

That two-part fix is exactly what #5751's "vendor-neutral device-domain model + optional plugin" section calls for -- this PoC is the evidence for why it's needed at both the HyperNode-gradient stage and the Node-predicate stage, not just one.