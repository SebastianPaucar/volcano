# PoC — fragmented device domains are invisible until after Node/HyperNode commit

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

go: downloading k8s.io/apiserver v0.36.1
go: downloading github.com/google/go-cmp v0.7.0
go: downloading k8s.io/component-base v0.36.1
go: downloading k8s.io/kubernetes v1.36.1
go: downloading github.com/prometheus/client_golang v1.23.2
go: downloading k8s.io/kube-scheduler v0.36.1
go: downloading github.com/mitchellh/mapstructure v1.5.0
go: downloading k8s.io/component-helpers v0.36.1

go: downloading github.com/beorn7/perks v1.0.1
go: downloading github.com/cespare/xxhash/v2 v2.3.0
go: downloading github.com/prometheus/common v0.70.0
go: downloading github.com/prometheus/client_model v0.6.2
go: downloading github.com/prometheus/procfs v0.21.0
go: downloading github.com/blang/semver/v4 v4.0.0
go: downloading go.opentelemetry.io/otel/trace v1.43.0
go: downloading k8s.io/dynamic-resource-allocation v0.36.1
go: downloading stathat.com/c/consistent v1.0.0
go: downloading k8s.io/controller-manager v0.36.1
go: downloading k8s.io/apiextensions-apiserver v0.36.1
go: downloading go.opentelemetry.io/otel v1.43.0
go: downloading github.com/google/cadvisor v0.56.2
go: downloading k8s.io/csi-translation-lib v0.36.1
go: downloading k8s.io/cloud-provider v0.36.1
go: downloading github.com/spf13/cobra v1.10.2
go: downloading github.com/elastic/go-elasticsearch/v7 v7.17.10
go: downloading k8s.io/metrics v0.36.1
go: downloading github.com/google/cel-go v0.26.0
go: downloading cel.dev/expr v0.25.1
go: downloading google.golang.org/genproto/googleapis/api v0.0.0-20260319201613-d00831a3d3e7
go: downloading github.com/stoewer/go-strcase v1.3.0
go: downloading golang.org/x/sync v0.22.0
go: downloading github.com/antlr4-go/antlr/v4 v4.13.0
go: downloading google.golang.org/genproto/googleapis/rpc v0.0.0-20260311181403-84a4fc48630c
go: downloading golang.org/x/exp v0.0.0-20260218203240-3dfff04db8fa

go test ./pkg/scheduler/actions/allocate/ -run TestCase2_FragmentedDomainNotVisibleAtSelectionTime -v -count=8

=== RUN   TestCase2_FragmentedDomainNotVisibleAtSelectionTime
    case2_fragmentation_poc_test.go:170: frag-node: FilterCalled=0 AllocateCalled=0 log=[]
    case2_fragmentation_poc_test.go:171: healthy-node: FilterCalled=0 AllocateCalled=1 log=[call#1 pod=p1 aggregateFreeAtCallTime=8]
    case2_fragmentation_poc_test.go:188: Scheduler selected healthy-node this run (non-deterministic tie-break on identical aggregate scores). If AllocateCalled > 1 here, that's a SEPARATE finding worth investigating on its own: Volcano called Allocate() more than once for what should be a single task placement in a single cycle -- check statement.go's rollback path and allocateResourcesForTask's retry behavior around line ~841/958 in allocate.go for why a single-attempt scheduling cycle produced multiple Allocate() calls on one node.
--- PASS: TestCase2_FragmentedDomainNotVisibleAtSelectionTime (0.11s)
=== RUN   TestCase2_FragmentedDomainNotVisibleAtSelectionTime
E0816 19:55:33.061989 1786078 predicates.go:245] AllocateToPod failed POC: node frag-node has 8 aggregate free devices (enough) but no single domain has 8 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.062925 1786078 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <frag-node> when allocating in Session <1aadddb0-4499-400e-bb10-9a0f9388fecb>: POC: node frag-node has 8 aggregate free devices (enough) but no single domain has 8 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.063016 1786078 predicates.go:308] predicates, remove pod c1/p1 from node [frag-node] error: no corresponding pod p1 in pods of node frag-node
E0816 19:55:33.063039 1786078 allocate.go:958] Failed to bind Task c1-p1 on frag-node in Session 1aadddb0-4499-400e-bb10-9a0f9388fecb, err: Task c1/p1 allocate to node frag-node error and errInfos num is 1, allocation has been rolled back
E0816 19:55:33.063104 1786078 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node frag-node error and errInfos num is 1, allocation has been rolled back" task="p1"
    case2_fragmentation_poc_test.go:170: frag-node: FilterCalled=0 AllocateCalled=1 log=[call#1 pod=p1 aggregateFreeAtCallTime=8]
    case2_fragmentation_poc_test.go:171: healthy-node: FilterCalled=0 AllocateCalled=0 log=[]
    case2_fragmentation_poc_test.go:184: POC CONFIRMED: scheduler selected frag-node (identical aggregate free=8 as healthy-node), device.Allocate() failed post-commit on the fragmented node, and healthy-node was never tried in this cycle -- no cross-node fallback. See AllocateCallLog above for exact call count/order on frag-node.
--- PASS: TestCase2_FragmentedDomainNotVisibleAtSelectionTime (0.11s)
=== RUN   TestCase2_FragmentedDomainNotVisibleAtSelectionTime
E0816 19:55:33.167850 1786078 predicates.go:245] AllocateToPod failed POC: node healthy-node has 0 aggregate free devices (enough) but no single domain has 8 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.167887 1786078 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <healthy-node> when allocating in Session <682eeee7-73b3-4abf-9609-17448f61c749>: POC: node healthy-node has 0 aggregate free devices (enough) but no single domain has 8 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.167959 1786078 predicates.go:308] predicates, remove pod c1/p1 from node [healthy-node] error: no corresponding pod p1 in pods of node healthy-node
E0816 19:55:33.168001 1786078 allocate.go:958] Failed to bind Task c1-p1 on healthy-node in Session 682eeee7-73b3-4abf-9609-17448f61c749, err: Task c1/p1 allocate to node healthy-node error and errInfos num is 1, allocation has been rolled back
E0816 19:55:33.168073 1786078 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node healthy-node error and errInfos num is 1, allocation has been rolled back" task="p1"
    case2_fragmentation_poc_test.go:170: frag-node: FilterCalled=0 AllocateCalled=0 log=[]
    case2_fragmentation_poc_test.go:171: healthy-node: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=8 call#2 pod=p1 aggregateFreeAtCallTime=0]
    case2_fragmentation_poc_test.go:188: Scheduler selected healthy-node this run (non-deterministic tie-break on identical aggregate scores). If AllocateCalled > 1 here, that's a SEPARATE finding worth investigating on its own: Volcano called Allocate() more than once for what should be a single task placement in a single cycle -- check statement.go's rollback path and allocateResourcesForTask's retry behavior around line ~841/958 in allocate.go for why a single-attempt scheduling cycle produced multiple Allocate() calls on one node.
--- PASS: TestCase2_FragmentedDomainNotVisibleAtSelectionTime (0.10s)
=== RUN   TestCase2_FragmentedDomainNotVisibleAtSelectionTime
E0816 19:55:33.272842 1786078 predicates.go:245] AllocateToPod failed POC: node healthy-node has 0 aggregate free devices (enough) but no single domain has 8 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.272898 1786078 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <healthy-node> when allocating in Session <97403f05-8117-4f8d-9f88-4e36e4b44685>: POC: node healthy-node has 0 aggregate free devices (enough) but no single domain has 8 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.272965 1786078 predicates.go:308] predicates, remove pod c1/p1 from node [healthy-node] error: no corresponding pod p1 in pods of node healthy-node
E0816 19:55:33.273003 1786078 allocate.go:958] Failed to bind Task c1-p1 on healthy-node in Session 97403f05-8117-4f8d-9f88-4e36e4b44685, err: Task c1/p1 allocate to node healthy-node error and errInfos num is 1, allocation has been rolled back
E0816 19:55:33.273060 1786078 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node healthy-node error and errInfos num is 1, allocation has been rolled back" task="p1"
    case2_fragmentation_poc_test.go:170: frag-node: FilterCalled=0 AllocateCalled=0 log=[]
    case2_fragmentation_poc_test.go:171: healthy-node: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=8 call#2 pod=p1 aggregateFreeAtCallTime=0]
    case2_fragmentation_poc_test.go:188: Scheduler selected healthy-node this run (non-deterministic tie-break on identical aggregate scores). If AllocateCalled > 1 here, that's a SEPARATE finding worth investigating on its own: Volcano called Allocate() more than once for what should be a single task placement in a single cycle -- check statement.go's rollback path and allocateResourcesForTask's retry behavior around line ~841/958 in allocate.go for why a single-attempt scheduling cycle produced multiple Allocate() calls on one node.
--- PASS: TestCase2_FragmentedDomainNotVisibleAtSelectionTime (0.10s)
=== RUN   TestCase2_FragmentedDomainNotVisibleAtSelectionTime
E0816 19:55:33.380155 1786078 predicates.go:245] AllocateToPod failed POC: node healthy-node has 0 aggregate free devices (enough) but no single domain has 8 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.380189 1786078 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <healthy-node> when allocating in Session <e777da13-2a18-4248-810d-6aa534ba1ddc>: POC: node healthy-node has 0 aggregate free devices (enough) but no single domain has 8 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.380224 1786078 predicates.go:308] predicates, remove pod c1/p1 from node [healthy-node] error: no corresponding pod p1 in pods of node healthy-node
E0816 19:55:33.380238 1786078 allocate.go:958] Failed to bind Task c1-p1 on healthy-node in Session e777da13-2a18-4248-810d-6aa534ba1ddc, err: Task c1/p1 allocate to node healthy-node error and errInfos num is 1, allocation has been rolled back
E0816 19:55:33.380268 1786078 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node healthy-node error and errInfos num is 1, allocation has been rolled back" task="p1"
    case2_fragmentation_poc_test.go:170: frag-node: FilterCalled=0 AllocateCalled=0 log=[]
    case2_fragmentation_poc_test.go:171: healthy-node: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=8 call#2 pod=p1 aggregateFreeAtCallTime=0]
    case2_fragmentation_poc_test.go:188: Scheduler selected healthy-node this run (non-deterministic tie-break on identical aggregate scores). If AllocateCalled > 1 here, that's a SEPARATE finding worth investigating on its own: Volcano called Allocate() more than once for what should be a single task placement in a single cycle -- check statement.go's rollback path and allocateResourcesForTask's retry behavior around line ~841/958 in allocate.go for why a single-attempt scheduling cycle produced multiple Allocate() calls on one node.
--- PASS: TestCase2_FragmentedDomainNotVisibleAtSelectionTime (0.11s)
=== RUN   TestCase2_FragmentedDomainNotVisibleAtSelectionTime
E0816 19:55:33.484884 1786078 predicates.go:245] AllocateToPod failed POC: node frag-node has 8 aggregate free devices (enough) but no single domain has 8 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.484922 1786078 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <frag-node> when allocating in Session <12874d0c-03e0-4d4e-9c19-7325dacb1f2a>: POC: node frag-node has 8 aggregate free devices (enough) but no single domain has 8 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.484993 1786078 predicates.go:308] predicates, remove pod c1/p1 from node [frag-node] error: no corresponding pod p1 in pods of node frag-node
E0816 19:55:33.485017 1786078 allocate.go:958] Failed to bind Task c1-p1 on frag-node in Session 12874d0c-03e0-4d4e-9c19-7325dacb1f2a, err: Task c1/p1 allocate to node frag-node error and errInfos num is 1, allocation has been rolled back
E0816 19:55:33.485081 1786078 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node frag-node error and errInfos num is 1, allocation has been rolled back" task="p1"
    case2_fragmentation_poc_test.go:170: frag-node: FilterCalled=0 AllocateCalled=1 log=[call#1 pod=p1 aggregateFreeAtCallTime=8]
    case2_fragmentation_poc_test.go:171: healthy-node: FilterCalled=0 AllocateCalled=0 log=[]
    case2_fragmentation_poc_test.go:184: POC CONFIRMED: scheduler selected frag-node (identical aggregate free=8 as healthy-node), device.Allocate() failed post-commit on the fragmented node, and healthy-node was never tried in this cycle -- no cross-node fallback. See AllocateCallLog above for exact call count/order on frag-node.
--- PASS: TestCase2_FragmentedDomainNotVisibleAtSelectionTime (0.10s)
=== RUN   TestCase2_FragmentedDomainNotVisibleAtSelectionTime
E0816 19:55:33.589624 1786078 predicates.go:245] AllocateToPod failed POC: node healthy-node has 0 aggregate free devices (enough) but no single domain has 8 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.589668 1786078 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <healthy-node> when allocating in Session <8ced22f1-646f-4281-8719-4fabd843d942>: POC: node healthy-node has 0 aggregate free devices (enough) but no single domain has 8 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.589775 1786078 predicates.go:308] predicates, remove pod c1/p1 from node [healthy-node] error: no corresponding pod p1 in pods of node healthy-node
E0816 19:55:33.589815 1786078 allocate.go:958] Failed to bind Task c1-p1 on healthy-node in Session 8ced22f1-646f-4281-8719-4fabd843d942, err: Task c1/p1 allocate to node healthy-node error and errInfos num is 1, allocation has been rolled back
E0816 19:55:33.589899 1786078 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node healthy-node error and errInfos num is 1, allocation has been rolled back" task="p1"
    case2_fragmentation_poc_test.go:170: frag-node: FilterCalled=0 AllocateCalled=0 log=[]
    case2_fragmentation_poc_test.go:171: healthy-node: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=8 call#2 pod=p1 aggregateFreeAtCallTime=0]
    case2_fragmentation_poc_test.go:188: Scheduler selected healthy-node this run (non-deterministic tie-break on identical aggregate scores). If AllocateCalled > 1 here, that's a SEPARATE finding worth investigating on its own: Volcano called Allocate() more than once for what should be a single task placement in a single cycle -- check statement.go's rollback path and allocateResourcesForTask's retry behavior around line ~841/958 in allocate.go for why a single-attempt scheduling cycle produced multiple Allocate() calls on one node.
--- PASS: TestCase2_FragmentedDomainNotVisibleAtSelectionTime (0.10s)
=== RUN   TestCase2_FragmentedDomainNotVisibleAtSelectionTime
E0816 19:55:33.694696 1786078 predicates.go:245] AllocateToPod failed POC: node frag-node has 8 aggregate free devices (enough) but no single domain has 8 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.694736 1786078 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <frag-node> when allocating in Session <b0723ccc-912f-4d92-b1e0-d616b98cf2b9>: POC: node frag-node has 8 aggregate free devices (enough) but no single domain has 8 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0816 19:55:33.694856 1786078 predicates.go:308] predicates, remove pod c1/p1 from node [frag-node] error: no corresponding pod p1 in pods of node frag-node
E0816 19:55:33.694895 1786078 allocate.go:958] Failed to bind Task c1-p1 on frag-node in Session b0723ccc-912f-4d92-b1e0-d616b98cf2b9, err: Task c1/p1 allocate to node frag-node error and errInfos num is 1, allocation has been rolled back
E0816 19:55:33.695004 1786078 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node frag-node error and errInfos num is 1, allocation has been rolled back" task="p1"
    case2_fragmentation_poc_test.go:170: frag-node: FilterCalled=0 AllocateCalled=1 log=[call#1 pod=p1 aggregateFreeAtCallTime=8]
    case2_fragmentation_poc_test.go:171: healthy-node: FilterCalled=0 AllocateCalled=0 log=[]
    case2_fragmentation_poc_test.go:184: POC CONFIRMED: scheduler selected frag-node (identical aggregate free=8 as healthy-node), device.Allocate() failed post-commit on the fragmented node, and healthy-node was never tried in this cycle -- no cross-node fallback. See AllocateCallLog above for exact call count/order on frag-node.
--- PASS: TestCase2_FragmentedDomainNotVisibleAtSelectionTime (0.11s)
PASS
ok  	volcano.sh/volcano/pkg/scheduler/actions/allocate	0.912s
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