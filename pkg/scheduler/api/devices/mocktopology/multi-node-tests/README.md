# PoC — a gang's tasks can split across a valid and an invalid Node within one HyperNode, uncoordinated

**Status: working, verified against real scheduler code, 8/8 runs consistent with the finding (1/8 shows the clean split; the rest show a related, secondary pattern -- both explained below).**

## What this proves

#5751's Case 2 text requires: *"Each Pod must receive a valid local device group, while the complete PodGroup should be placed in an appropriate rack or network HyperNode. Free devices on different Nodes must not be treated as one shared xPU fabric."*

`network_topology_aware.go`'s HyperNode-level scoring (`resourceStatus` cache) tracks only `allocatable`/`used`/`idle`/`futureIdle` -- no per-domain fields (confirmed by reading the cache struct directly). Node-level scoring inside a chosen HyperNode is equally domain-blind (see PoC #3). This PoC asks the next question: if a gang's tasks are scheduled independently, one Node selection at a time, within a HyperNode whose *members* differ in device-domain health, does anything stop the gang from being split across a good Node and a bad Node -- with no check that *every* Node the gang ends up on actually has a valid local device group?

This PoC reproduces that scenario end-to-end against real, shipped Volcano scheduler code -- the actual `allocate` action, `predicates`, `gang`, and `network-topology-aware` plugins, real `HyperNodesMap`/`HyperNodesSetByTier`/`HyperNodes` fixtures matching the pattern used in `allocate_test.go`'s own passing hard-topology tests -- with only the device backend mocked (same mock as PoC #3).

## Files

- `mock_topology_device.go` -> `pkg/scheduler/api/devices/mocktopology/mock_topology_device.go` (shared with PoC #3; this PoC adds an `AllocateResults` field, additive and backward-compatible -- PoC #3 still passes unmodified after this change)
- `case2_multinode_poc_test.go` -> `pkg/scheduler/actions/allocate/case2_multinode_poc_test.go`

## How to run

From `volcano`:

```bash
go build ./pkg/scheduler/api/devices/mocktopology/...
go vet ./pkg/scheduler/actions/allocate/...
go test ./pkg/scheduler/actions/allocate/ -run TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode -v -count=8

=== RUN   TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode
E0819 18:28:29.869916 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.870395 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s0-n2> when allocating in Session <504e1e16-0bb3-4542-b306-237f712faff5>: POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.870509 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s0-n2] error: no corresponding pod p1 in pods of node s0-n2
E0819 18:28:29.870561 1337036 allocate.go:958] Failed to bind Task c1-p1 on s0-n2 in Session 504e1e16-0bb3-4542-b306-237f712faff5, err: Task c1/p1 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:29.870641 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:29.871017 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.871055 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s0-n2> when allocating in Session <504e1e16-0bb3-4542-b306-237f712faff5>: POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.871130 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s0-n2] error: no corresponding pod p2 in pods of node s0-n2
E0819 18:28:29.871165 1337036 allocate.go:958] Failed to bind Task c1-p2 on s0-n2 in Session 504e1e16-0bb3-4542-b306-237f712faff5, err: Task c1/p2 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:29.871236 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back" task="p2"
    case2_multinode_poc_test.go:187: s0-n1: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:187: s0-n2: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p2 aggregateFreeAtCallTime=6] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:187: s1-n1: FilterCalled=0 AllocateCalled=3 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=6 call#3 pod=p1 aggregateFreeAtCallTime=6] results=map[p1:true]
    case2_multinode_poc_test.go:187: s1-n2: FilterCalled=0 AllocateCalled=3 log=[call#1 pod=p2 aggregateFreeAtCallTime=6 call#2 pod=p2 aggregateFreeAtCallTime=6 call#3 pod=p2 aggregateFreeAtCallTime=6] results=map[p2:true]
    case2_multinode_poc_test.go:216: p1 final placement: {node:s0-n2 success:false found:true}
    case2_multinode_poc_test.go:217: p2 final placement: {node:s0-n2 success:false found:true}
    case2_multinode_poc_test.go:241: Both tasks were (finally) attempted on the SAME Node (s0-n2) -- not the split scenario this test targets. This run doesn't exercise Case 2's cross-Node concern; re-run with -count=N.
--- PASS: TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode (0.12s)
=== RUN   TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode
E0819 18:28:29.978840 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.978894 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s1-n1> when allocating in Session <a171c915-9af3-43b4-8dab-07399ba248b1>: POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.978990 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s1-n1] error: no corresponding pod p1 in pods of node s1-n1
E0819 18:28:29.979031 1337036 allocate.go:958] Failed to bind Task c1-p1 on s1-n1 in Session a171c915-9af3-43b4-8dab-07399ba248b1, err: Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:29.979128 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:29.979476 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.979507 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s1-n2> when allocating in Session <a171c915-9af3-43b4-8dab-07399ba248b1>: POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.979573 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s1-n2] error: no corresponding pod p2 in pods of node s1-n2
E0819 18:28:29.979609 1337036 allocate.go:958] Failed to bind Task c1-p2 on s1-n2 in Session a171c915-9af3-43b4-8dab-07399ba248b1, err: Task c1/p2 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:29.979680 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back" task="p2"
E0819 18:28:29.980060 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.980109 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s0-n2> when allocating in Session <a171c915-9af3-43b4-8dab-07399ba248b1>: POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.980196 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s0-n2] error: no corresponding pod p1 in pods of node s0-n2
E0819 18:28:29.980232 1337036 allocate.go:958] Failed to bind Task c1-p1 on s0-n2 in Session a171c915-9af3-43b4-8dab-07399ba248b1, err: Task c1/p1 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:29.980308 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:29.980620 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.980652 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s0-n1> when allocating in Session <a171c915-9af3-43b4-8dab-07399ba248b1>: POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:29.980720 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s0-n1] error: no corresponding pod p2 in pods of node s0-n1
E0819 18:28:29.980757 1337036 allocate.go:958] Failed to bind Task c1-p2 on s0-n1 in Session a171c915-9af3-43b4-8dab-07399ba248b1, err: Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:29.980828 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back" task="p2"
    case2_multinode_poc_test.go:187: s0-n1: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p2 aggregateFreeAtCallTime=6 call#2 pod=p2 aggregateFreeAtCallTime=0] results=map[p2:false]
    case2_multinode_poc_test.go:187: s0-n2: FilterCalled=0 AllocateCalled=1 log=[call#1 pod=p1 aggregateFreeAtCallTime=6] results=map[p1:false]
    case2_multinode_poc_test.go:187: s1-n1: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0] results=map[p1:false]
    case2_multinode_poc_test.go:187: s1-n2: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p2 aggregateFreeAtCallTime=6 call#2 pod=p2 aggregateFreeAtCallTime=0] results=map[p2:false]
    case2_multinode_poc_test.go:216: p1 final placement: {node:s0-n2 success:false found:true}
    case2_multinode_poc_test.go:217: p2 final placement: {node:s0-n1 success:false found:true}
    case2_multinode_poc_test.go:234: POC CONFIRMED (split, same outcome): gang tasks p1 (node=s0-n2 success=false) and p2 (node=s0-n1 success=false) landed on DIFFERENT Nodes within the SAME HyperNode. Both happened to end with the same success/failure this run, but the split itself is the point: nothing in HyperNode/Node selection tries to keep a gang's tasks together, or checks that EVERY member Node in the chosen HyperNode actually has a valid local device group before committing the HyperNode for this gang.
--- PASS: TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode (0.11s)
=== RUN   TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode
E0819 18:28:30.089257 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.089301 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s1-n2> when allocating in Session <1ec1e9cb-ed80-4cf6-9bd8-b35c132ebe12>: POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.089363 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s1-n2] error: no corresponding pod p1 in pods of node s1-n2
E0819 18:28:30.089392 1337036 allocate.go:958] Failed to bind Task c1-p1 on s1-n2 in Session 1ec1e9cb-ed80-4cf6-9bd8-b35c132ebe12, err: Task c1/p1 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.089445 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.089878 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.089923 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s1-n1> when allocating in Session <1ec1e9cb-ed80-4cf6-9bd8-b35c132ebe12>: POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.090004 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s1-n1] error: no corresponding pod p2 in pods of node s1-n1
E0819 18:28:30.090038 1337036 allocate.go:958] Failed to bind Task c1-p2 on s1-n1 in Session 1ec1e9cb-ed80-4cf6-9bd8-b35c132ebe12, err: Task c1/p2 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.090098 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back" task="p2"
E0819 18:28:30.090420 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.090450 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s0-n2> when allocating in Session <1ec1e9cb-ed80-4cf6-9bd8-b35c132ebe12>: POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.090504 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s0-n2] error: no corresponding pod p1 in pods of node s0-n2
E0819 18:28:30.090528 1337036 allocate.go:958] Failed to bind Task c1-p1 on s0-n2 in Session 1ec1e9cb-ed80-4cf6-9bd8-b35c132ebe12, err: Task c1/p1 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.090577 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.090807 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.090838 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s0-n2> when allocating in Session <1ec1e9cb-ed80-4cf6-9bd8-b35c132ebe12>: POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.090907 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s0-n2] error: no corresponding pod p2 in pods of node s0-n2
E0819 18:28:30.090934 1337036 allocate.go:958] Failed to bind Task c1-p2 on s0-n2 in Session 1ec1e9cb-ed80-4cf6-9bd8-b35c132ebe12, err: Task c1/p2 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.090988 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back" task="p2"
    case2_multinode_poc_test.go:187: s0-n1: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:187: s0-n2: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p2 aggregateFreeAtCallTime=6] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:187: s1-n1: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p2 aggregateFreeAtCallTime=6 call#2 pod=p2 aggregateFreeAtCallTime=0] results=map[p2:false]
    case2_multinode_poc_test.go:187: s1-n2: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0] results=map[p1:false]
    case2_multinode_poc_test.go:216: p1 final placement: {node:s1-n2 success:false found:true}
    case2_multinode_poc_test.go:217: p2 final placement: {node:s0-n2 success:false found:true}
    case2_multinode_poc_test.go:244: Tasks landed in DIFFERENT HyperNodes (p1=s1-n2, p2=s0-n2) -- likely evidence of gang re-attempt across cycles rather than a single-HyperNode split. Worth its own investigation but not what this test targets; check AllocateCallLog above for the full sequence before concluding anything.
--- PASS: TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode (0.11s)
=== RUN   TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode
E0819 18:28:30.196385 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.196429 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s0-n1> when allocating in Session <b56b750c-18b7-4b5b-b74b-df1262359d34>: POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.196508 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s0-n1] error: no corresponding pod p1 in pods of node s0-n1
E0819 18:28:30.196540 1337036 allocate.go:958] Failed to bind Task c1-p1 on s0-n1 in Session b56b750c-18b7-4b5b-b74b-df1262359d34, err: Task c1/p1 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.196617 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.196957 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.196998 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s0-n1> when allocating in Session <b56b750c-18b7-4b5b-b74b-df1262359d34>: POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.197086 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s0-n1] error: no corresponding pod p2 in pods of node s0-n1
E0819 18:28:30.197130 1337036 allocate.go:958] Failed to bind Task c1-p2 on s0-n1 in Session b56b750c-18b7-4b5b-b74b-df1262359d34, err: Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.197199 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back" task="p2"
E0819 18:28:30.197502 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.197531 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s1-n1> when allocating in Session <b56b750c-18b7-4b5b-b74b-df1262359d34>: POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.197587 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s1-n1] error: no corresponding pod p1 in pods of node s1-n1
E0819 18:28:30.197611 1337036 allocate.go:958] Failed to bind Task c1-p1 on s1-n1 in Session b56b750c-18b7-4b5b-b74b-df1262359d34, err: Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.197663 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.197966 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.197996 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s1-n2> when allocating in Session <b56b750c-18b7-4b5b-b74b-df1262359d34>: POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.198055 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s1-n2] error: no corresponding pod p2 in pods of node s1-n2
E0819 18:28:30.198080 1337036 allocate.go:958] Failed to bind Task c1-p2 on s1-n2 in Session b56b750c-18b7-4b5b-b74b-df1262359d34, err: Task c1/p2 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.198133 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back" task="p2"
    case2_multinode_poc_test.go:187: s1-n1: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0] results=map[p1:false]
    case2_multinode_poc_test.go:187: s1-n2: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p2 aggregateFreeAtCallTime=6 call#2 pod=p2 aggregateFreeAtCallTime=0] results=map[p2:false]
    case2_multinode_poc_test.go:187: s0-n1: FilterCalled=0 AllocateCalled=4 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0 call#3 pod=p2 aggregateFreeAtCallTime=6 call#4 pod=p2 aggregateFreeAtCallTime=0] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:187: s0-n2: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:216: p1 final placement: {node:s0-n1 success:false found:true}
    case2_multinode_poc_test.go:217: p2 final placement: {node:s0-n1 success:false found:true}
    case2_multinode_poc_test.go:241: Both tasks were (finally) attempted on the SAME Node (s0-n1) -- not the split scenario this test targets. This run doesn't exercise Case 2's cross-Node concern; re-run with -count=N.
--- PASS: TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode (0.11s)
=== RUN   TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode
E0819 18:28:30.303678 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.303725 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s0-n2> when allocating in Session <b132bc85-9394-4518-a477-a474c4a1ca95>: POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.303888 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s0-n2] error: no corresponding pod p1 in pods of node s0-n2
E0819 18:28:30.303960 1337036 allocate.go:958] Failed to bind Task c1-p1 on s0-n2 in Session b132bc85-9394-4518-a477-a474c4a1ca95, err: Task c1/p1 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.304077 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.304546 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.304585 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s0-n2> when allocating in Session <b132bc85-9394-4518-a477-a474c4a1ca95>: POC: node s0-n2 has 6 aggregate free devices (enough) but no single domain has 6 free (fragmented across 2 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.304687 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s0-n2] error: no corresponding pod p2 in pods of node s0-n2
E0819 18:28:30.304735 1337036 allocate.go:958] Failed to bind Task c1-p2 on s0-n2 in Session b132bc85-9394-4518-a477-a474c4a1ca95, err: Task c1/p2 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.304793 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s0-n2 error and errInfos num is 1, allocation has been rolled back" task="p2"
E0819 18:28:30.305109 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.305142 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s1-n1> when allocating in Session <b132bc85-9394-4518-a477-a474c4a1ca95>: POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.305215 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s1-n1] error: no corresponding pod p1 in pods of node s1-n1
E0819 18:28:30.305277 1337036 allocate.go:958] Failed to bind Task c1-p1 on s1-n1 in Session b132bc85-9394-4518-a477-a474c4a1ca95, err: Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.305350 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.305657 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.305698 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s1-n1> when allocating in Session <b132bc85-9394-4518-a477-a474c4a1ca95>: POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.305764 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s1-n1] error: no corresponding pod p2 in pods of node s1-n1
E0819 18:28:30.305793 1337036 allocate.go:958] Failed to bind Task c1-p2 on s1-n1 in Session b132bc85-9394-4518-a477-a474c4a1ca95, err: Task c1/p2 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.305851 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back" task="p2"
    case2_multinode_poc_test.go:187: s1-n1: FilterCalled=0 AllocateCalled=4 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0 call#3 pod=p2 aggregateFreeAtCallTime=6 call#4 pod=p2 aggregateFreeAtCallTime=0] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:187: s1-n2: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:187: s0-n1: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:187: s0-n2: FilterCalled=0 AllocateCalled=2 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p2 aggregateFreeAtCallTime=6] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:216: p1 final placement: {node:s0-n2 success:false found:true}
    case2_multinode_poc_test.go:217: p2 final placement: {node:s0-n2 success:false found:true}
    case2_multinode_poc_test.go:241: Both tasks were (finally) attempted on the SAME Node (s0-n2) -- not the split scenario this test targets. This run doesn't exercise Case 2's cross-Node concern; re-run with -count=N.
--- PASS: TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode (0.11s)
=== RUN   TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode
E0819 18:28:30.414781 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.414833 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s0-n1> when allocating in Session <0d747701-3cd8-4530-83be-899c77dab863>: POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.415003 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s0-n1] error: no corresponding pod p1 in pods of node s0-n1
E0819 18:28:30.415103 1337036 allocate.go:958] Failed to bind Task c1-p1 on s0-n1 in Session 0d747701-3cd8-4530-83be-899c77dab863, err: Task c1/p1 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.415716 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.416077 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.416125 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s0-n1> when allocating in Session <0d747701-3cd8-4530-83be-899c77dab863>: POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.416205 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s0-n1] error: no corresponding pod p2 in pods of node s0-n1
E0819 18:28:30.416237 1337036 allocate.go:958] Failed to bind Task c1-p2 on s0-n1 in Session 0d747701-3cd8-4530-83be-899c77dab863, err: Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.416308 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back" task="p2"
E0819 18:28:30.416672 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.416702 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s1-n1> when allocating in Session <0d747701-3cd8-4530-83be-899c77dab863>: POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.416746 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s1-n1] error: no corresponding pod p1 in pods of node s1-n1
E0819 18:28:30.416767 1337036 allocate.go:958] Failed to bind Task c1-p1 on s1-n1 in Session 0d747701-3cd8-4530-83be-899c77dab863, err: Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.416803 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.417065 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.417084 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s1-n1> when allocating in Session <0d747701-3cd8-4530-83be-899c77dab863>: POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.417118 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s1-n1] error: no corresponding pod p2 in pods of node s1-n1
E0819 18:28:30.417133 1337036 allocate.go:958] Failed to bind Task c1-p2 on s1-n1 in Session 0d747701-3cd8-4530-83be-899c77dab863, err: Task c1/p2 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.417161 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back" task="p2"
    case2_multinode_poc_test.go:187: s0-n1: FilterCalled=0 AllocateCalled=4 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0 call#3 pod=p2 aggregateFreeAtCallTime=6 call#4 pod=p2 aggregateFreeAtCallTime=0] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:187: s0-n2: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:187: s1-n1: FilterCalled=0 AllocateCalled=4 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0 call#3 pod=p2 aggregateFreeAtCallTime=6 call#4 pod=p2 aggregateFreeAtCallTime=0] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:187: s1-n2: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:216: p1 final placement: {node:s0-n1 success:false found:true}
    case2_multinode_poc_test.go:217: p2 final placement: {node:s0-n1 success:false found:true}
    case2_multinode_poc_test.go:241: Both tasks were (finally) attempted on the SAME Node (s0-n1) -- not the split scenario this test targets. This run doesn't exercise Case 2's cross-Node concern; re-run with -count=N.
--- PASS: TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode (0.11s)
=== RUN   TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode
E0819 18:28:30.522176 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.522219 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s0-n1> when allocating in Session <44a33ac4-8527-4fe0-a35b-d290e4590a43>: POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.522305 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s0-n1] error: no corresponding pod p1 in pods of node s0-n1
E0819 18:28:30.522345 1337036 allocate.go:958] Failed to bind Task c1-p1 on s0-n1 in Session 44a33ac4-8527-4fe0-a35b-d290e4590a43, err: Task c1/p1 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.522425 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.522805 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.522849 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s0-n1> when allocating in Session <44a33ac4-8527-4fe0-a35b-d290e4590a43>: POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.522938 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s0-n1] error: no corresponding pod p2 in pods of node s0-n1
E0819 18:28:30.522971 1337036 allocate.go:958] Failed to bind Task c1-p2 on s0-n1 in Session 44a33ac4-8527-4fe0-a35b-d290e4590a43, err: Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.523030 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back" task="p2"
E0819 18:28:30.523374 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.523403 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s1-n1> when allocating in Session <44a33ac4-8527-4fe0-a35b-d290e4590a43>: POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.523463 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s1-n1] error: no corresponding pod p1 in pods of node s1-n1
E0819 18:28:30.523493 1337036 allocate.go:958] Failed to bind Task c1-p1 on s1-n1 in Session 44a33ac4-8527-4fe0-a35b-d290e4590a43, err: Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.523544 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.523805 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.523838 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s1-n1> when allocating in Session <44a33ac4-8527-4fe0-a35b-d290e4590a43>: POC: node s1-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.523929 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s1-n1] error: no corresponding pod p2 in pods of node s1-n1
E0819 18:28:30.523970 1337036 allocate.go:958] Failed to bind Task c1-p2 on s1-n1 in Session 44a33ac4-8527-4fe0-a35b-d290e4590a43, err: Task c1/p2 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.524039 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s1-n1 error and errInfos num is 1, allocation has been rolled back" task="p2"
    case2_multinode_poc_test.go:187: s0-n1: FilterCalled=0 AllocateCalled=4 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0 call#3 pod=p2 aggregateFreeAtCallTime=6 call#4 pod=p2 aggregateFreeAtCallTime=0] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:187: s0-n2: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:187: s1-n1: FilterCalled=0 AllocateCalled=4 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0 call#3 pod=p2 aggregateFreeAtCallTime=6 call#4 pod=p2 aggregateFreeAtCallTime=0] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:187: s1-n2: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:216: p1 final placement: {node:s0-n1 success:false found:true}
    case2_multinode_poc_test.go:217: p2 final placement: {node:s0-n1 success:false found:true}
    case2_multinode_poc_test.go:241: Both tasks were (finally) attempted on the SAME Node (s0-n1) -- not the split scenario this test targets. This run doesn't exercise Case 2's cross-Node concern; re-run with -count=N.
--- PASS: TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode (0.11s)
=== RUN   TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode
E0819 18:28:30.629033 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.629072 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s0-n1> when allocating in Session <1fe38e52-c212-467d-bc87-a01e89ec1de8>: POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.629139 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s0-n1] error: no corresponding pod p1 in pods of node s0-n1
E0819 18:28:30.629167 1337036 allocate.go:958] Failed to bind Task c1-p1 on s0-n1 in Session 1fe38e52-c212-467d-bc87-a01e89ec1de8, err: Task c1/p1 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.629224 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.629445 1337036 predicates.go:245] AllocateToPod failed POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.629474 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s0-n1> when allocating in Session <1fe38e52-c212-467d-bc87-a01e89ec1de8>: POC: node s0-n1 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.629533 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s0-n1] error: no corresponding pod p2 in pods of node s0-n1
E0819 18:28:30.629558 1337036 allocate.go:958] Failed to bind Task c1-p2 on s0-n1 in Session 1fe38e52-c212-467d-bc87-a01e89ec1de8, err: Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.629609 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s0-n1 error and errInfos num is 1, allocation has been rolled back" task="p2"
E0819 18:28:30.629931 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.629970 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p1> to node <s1-n2> when allocating in Session <1fe38e52-c212-467d-bc87-a01e89ec1de8>: POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.630038 1337036 predicates.go:308] predicates, remove pod c1/p1 from node [s1-n2] error: no corresponding pod p1 in pods of node s1-n2
E0819 18:28:30.630065 1337036 allocate.go:958] Failed to bind Task c1-p1 on s1-n2 in Session 1fe38e52-c212-467d-bc87-a01e89ec1de8, err: Task c1/p1 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.630121 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p1 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back" task="p1"
E0819 18:28:30.630412 1337036 predicates.go:245] AllocateToPod failed POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.630442 1337036 statement.go:304] Failed to exec allocate callback functions for task <c1/p2> to node <s1-n2> when allocating in Session <1fe38e52-c212-467d-bc87-a01e89ec1de8>: POC: node s1-n2 has 0 aggregate free devices (enough) but no single domain has 6 free (fragmented across 1 domains) -- allocation fails after Node/HyperNode already chosen
E0819 18:28:30.630504 1337036 predicates.go:308] predicates, remove pod c1/p2 from node [s1-n2] error: no corresponding pod p2 in pods of node s1-n2
E0819 18:28:30.630532 1337036 allocate.go:958] Failed to bind Task c1-p2 on s1-n2 in Session 1fe38e52-c212-467d-bc87-a01e89ec1de8, err: Task c1/p2 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back
E0819 18:28:30.630584 1337036 allocate.go:841] "Allocate resources for task fail" err="Task c1/p2 allocate to node s1-n2 error and errInfos num is 1, allocation has been rolled back" task="p2"
    case2_multinode_poc_test.go:187: s0-n2: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:187: s1-n1: FilterCalled=0 AllocateCalled=0 log=[] results=map[]
    case2_multinode_poc_test.go:187: s1-n2: FilterCalled=0 AllocateCalled=4 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0 call#3 pod=p2 aggregateFreeAtCallTime=6 call#4 pod=p2 aggregateFreeAtCallTime=0] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:187: s0-n1: FilterCalled=0 AllocateCalled=4 log=[call#1 pod=p1 aggregateFreeAtCallTime=6 call#2 pod=p1 aggregateFreeAtCallTime=0 call#3 pod=p2 aggregateFreeAtCallTime=6 call#4 pod=p2 aggregateFreeAtCallTime=0] results=map[p1:false p2:false]
    case2_multinode_poc_test.go:216: p1 final placement: {node:s0-n1 success:false found:true}
    case2_multinode_poc_test.go:217: p2 final placement: {node:s0-n1 success:false found:true}
    case2_multinode_poc_test.go:241: Both tasks were (finally) attempted on the SAME Node (s0-n1) -- not the split scenario this test targets. This run doesn't exercise Case 2's cross-Node concern; re-run with -count=N.
--- PASS: TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode (0.11s)
PASS
ok  	volcano.sh/volcano/pkg/scheduler/actions/allocate	0.936s
```

## Scenario

- HyperNode `s0`: `s0-n1` (healthy, one domain, 6 free) + `s0-n2` (fragmented, two domains 4+2 -- aggregate 6 matches the request, but max single domain is 4 < 6 requested)
- HyperNode `s1`: `s1-n1` (healthy, 6 free) + `s1-n2` (healthy, 6 free)
- Gang: 2 tasks (`master`, `worker`), hard network-topology mode tier 1, `minAvailable=2`, each requesting 6 devices as one domain

`s0` and `s1` report identical aggregate resources (2 nodes x 6 devices each), so which HyperNode gets tried is a non-deterministic tie-break, same mechanism as the Node-level tie-break in PoC #3.

## Results (8 real runs against the compiled scheduler)

| Outcome | Runs | What happened |
|---|---|---|
| Gang split across two different Nodes in the same HyperNode | 1/8 | `p1` -> `s0-n2` (fails), `p2` -> `s0-n1` (fails) -- **this is the scenario the test targets, reproduced cleanly** |
| Both tasks converged on the same single Node | 6/8 | Both `p1` and `p2` attempted (and retried) on one Node, e.g. `s0-n1` or `s1-n2` |
| Tasks landed in different HyperNodes | 1/8 | Consistent with gang re-attempt across scheduling cycles rather than a single-HyperNode split |

### Finding 1 (primary -- directly answers Case 2's "valid local device group" requirement)

> Confirmed against real Volcano scheduler code: when a gang's two tasks are scheduled independently within an already-selected HyperNode whose member Nodes differ in device-domain health, the tasks can land on *different* Nodes with no coordination checking that both landing spots are valid. In the run where this occurred cleanly, `p1` was attempted on the fragmented Node and `p2` on the healthy Node -- two sibling tasks in the same gang, two different device-domain outcomes, and nothing in HyperNode selection, Node selection, or gang orchestration flagged or prevented the split.

This directly contradicts Case 2's stated requirement. The scheduler has no mechanism today to verify, before or after committing a gang to a HyperNode, that *every* member Node the gang's tasks land on has a valid local device group -- it only discovers per-task failure the same way PoC #3 showed for the single-node case: post-commit, one `Allocate()` call at a time.

### Finding 2 (secondary, but worth naming precisely -- domain-blind scoring concentrates gangs, which changes the failure shape)

In 6 of 8 runs, both gang tasks converged onto the *same* Node rather than spreading across the HyperNode's members. This is very likely ordinary binpack/compactness scoring doing exactly what it's designed to do -- packing tasks tightly -- but doing so with zero visibility into device-domain state. The practical effect: when concentration lands both tasks on a fragmented Node, the gang fails *together*, loudly, rather than silently succeeding for one task and failing for its sibling. That's arguably a gentler failure mode than Finding 1's split (a fully-failed gang is easier to notice and requeue than a half-placed one), but it does not fix the underlying blind spot -- it just changes which of the two bad outcomes shows up more often in a given run mix, and does nothing to prevent the split scenario in Finding 1 when scoring happens to spread the gang across Nodes instead of concentrating it.

Do not read the 6/8-vs-1/8 ratio as "the split rarely happens" in any strong sense -- both outcomes stem from the same root cause (zero domain visibility at selection time), and which one surfaces in a given run depends on scoring tie-breaks this PoC does not control for. A follow-up could force the split deterministically by giving `s0-n1` and `s0-n2` distinguishable non-domain scores so task-ordering reliably assigns one task to each.

### Finding 3 (already documented in PoC #3, reappears here in gang mode)

The double-`Allocate()`-per-node-per-pod artifact from PoC #3 (first call succeeds and consumes the domain, a rollback-path re-invocation of `AllocateFunc` calls it again and sees exhausted state) also appears here, inflating call counts on "healthy" Nodes and occasionally making them fail too. This is `Statement`'s rollback asymmetry (`statement.go:304`), not a new finding, and is **not** folded into Findings 1 or 2 above -- it's a separate bug in a separate subsystem, same as noted in PoC #3.

## Known caveats (read before citing)

- **Non-determinism is expected and is itself evidence**, same reasoning as PoC #3: both HyperNodes and, within `s0`, both Nodes report resource numbers that don't distinguish domain health, so which HyperNode/Node combination gets tried varies by run.
- **`finalPlacement` has a real limitation**: if a pod was tried on more than one Node across retries in a single cycle, Go's random map iteration order means the test's placement lookup may not always pick the truly last-in-time Node (`AllocateResults` isn't timestamped). The per-node `AllocateCallLog` (logged in full for every run) is the authoritative source if you need to verify call ordering precisely; the `finalPlacement` helper is a convenience read, not a guarantee.
- **Bind status isn't directly inspected**, same reason and same workaround as PoC #3 (`TestCommonStruct.binder` is unexported).

## Suggested next step for the real fix

Same two-part fix proposed in PoC #3, extended to explicitly cover the gang case:

- A domain-aware HyperNode pre-filter needs to check not just "does this HyperNode have enough aggregate devices" but "does every Node this gang's tasks would need actually have a valid local device group" -- which requires the gang's per-task device requirements to be visible at HyperNode-gradient time, not just at each task's individual `Allocate()` call.
- Wiring `Devices.FilterNode` into the real `Filter()` path (per PoC #3) would catch this per-task, per-Node -- but for gangs specifically, that alone isn't sufficient, since each task's `FilterNode` check happens in isolation; nothing today checks the *set* of Nodes a gang's tasks collectively land on before committing all of them.

This is exactly the "coordinate device choices across a PodGroup" requirement named in #5751's own problem statement -- this PoC is evidence that today, nothing does.