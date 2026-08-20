# PoC studies for #5751 — xPU topology-aware scheduling

Standalone studies tracing #5751. None of these is a PR. Each is scoped narrowly on purpose, and each README states plainly what it does and does not prove. Written in the course of applying for [#5751](https://github.com/volcano-sh/volcano/issues/5751) (CNCF LFX Mentorship, Term 3 2026).

---

## 1. `study/xpu-domain-mindcluster/` — vendor code has no domain concept to enforce

Calls the real, unmodified `NPUDevices.SelectChipFromNode` from `ascend/mindcluster/ascend310p/vnpu` twice: once with no domain restriction (it returns chips split across two domains for a request that needs one contiguous domain — real cross-domain placement, from real code), and once with the same call scoped externally to one domain at a time (both domains correctly refuse, since neither alone has enough chips).

**What it shows:** supplying domain membership from outside the vendor package, and scoping the vendor call to one domain at a time, is enough to
get correct behavior from a vendor plugin that has no domain concept of its own — no changes to the vendor code required.

[→ study/xpu-domain-mindcluster/README.md](./xpu-domain-mindcluster/README.md)

---

## 2. `cmd/domain-repro/` — the aggregate/domain distinction, made concrete against real decoding

A standalone program that decodes a node's device-register annotation using Volcano's real `devices.UnMarshalNodeDevices`, then computes two numbers side by side: total healthy devices on the node, and the largest single domain's healthy device count (domain membership read from the same `NetworkID` field the Ascend/HAMi plugin already uses). With a 5/5 domain split and an 8-device request, the two numbers disagree — aggregate says schedulable, domain says not.

**What it shows:** the aggregate-vs-domain gap described in #5751's own Case 1 text, restated in runnable form against Volcano's real annotation decoder rather than as prose.

[→ cmd/domain-repro/README.md](../cmd/domain-repro/README.md)

---

## 3. `pkg/scheduler/api/devices/mocktopology/` — the generic scheduler path has no enforcement point, not just the vendor layer

A mock `Devices` backend (satisfying Volcano's real `api.Devices` interface) wired into two nodes with identical aggregate free-device counts but different physical layouts — one node fragmented across two domains, one healthy — run end-to-end through the real, compiled `allocate` action, `predicates` plugin, and `gang` plugin (only the device backend is mocked; everything else is shipped scheduler code). Run 8 times to observe the non-deterministic Node-selection tie-break, since both nodes report identical aggregate counts.

**What it shows:** `Devices.FilterNode` — the hook that could reject a fragmented node before commitment — is wired only into preemption dry-run (`AddSimulatePredicateFn`), never into the real `Predicate()` path that gates actual Node selection. Real device allocation only runs *after* Node/HyperNode are already committed; if it fails there, the task is simply left unbound for that cycle, with no retry against a different Node. In 3 of 8 runs where the fragmented node was selected, this was reproduced cleanly. A secondary, unrelated observation surfaced in 4 of 8 runs — a partial/asymmetric rollback in `Statement`'s discard path.

[→ pkg/scheduler/api/devices/mocktopology/README.md](../pkg/scheduler/api/devices/mocktopology/README.md)

---

## 4. `pkg/scheduler/actions/allocate/case2_multinode_poc_test.go` — a gang's tasks can split across a valid and an invalid Node, uncoordinated
 
Extends the same mock device backend from study #3 into a two-HyperNode, multi-node, gang scenario (real `HyperNodesMap`/`HyperNodesSetByTier`/`HyperNodes` fixtures, same pattern as `allocate_test.go`'s own hard-topology tests). One HyperNode has one healthy Node and one fragmented Node; each gang task requests a full-domain device count independently. Run 8 times against the real, compiled `allocate`, `predicates`, `gang`, and `network-topology-aware` plugins.
 
**What it shows:** in the run where the gang split across the HyperNode's two Nodes, one sibling task landed on the healthy Node and the other on the fragmented one — with nothing in HyperNode selection, Node selection, or gang orchestration checking that *every* Node a gang's tasks land on actually has a valid local device group. This directly targets #5751 Case 2's own language ("each Pod must receive a valid local device group... coordinate device choices across a PodGroup"). In most runs the two tasks instead converged onto the *same* Node — a related, secondary pattern worth naming on its own: domain-blind binpack scoring concentrates gang tasks together, which changes the shape of the failure (the gang fails as a whole, more visibly) without fixing the blind spot that causes it.
 
[→ pkg/scheduler/api/devices/mocktopology/README-case2-multinode.md](../pkg/scheduler/api/devices/mocktopology/multi-node-tests/README.md)

---

## How these PoCs fit together

Read in combination, not isolation:

* Study #1 shows a specific vendor has no domain concept to enforce
* Study #2 makes the resulting aggregate/domain disagreement concrete against real decoding logic
* Study #3 shows that even where a domain-check hook exists in the generic scheduler, it isn't wired into the path that actually decides Node placement.
* Study #4 extends #3 to gangs across multiple Nodes and HyperNodes, showing the same blind spot lets a gang's tasks split across good and bad hardware with no coordination — the exact multi-Node coordination gap #5751's Case 2 describes


These are Different layers of the same gap — the data model, one vendor's implementation, and the scheduler's own orchestration timing — none of which alone would motivate #5751's proposed generic, vendor-neutral topology model as clearly as seeing all three together.
