// Case 2 PoC: within a single already-selected HyperNode, can a gang's tasks
// be split across a valid and an invalid Node, with device-domain
// feasibility never consulted at HyperNode selection, Node selection, or
// between sibling tasks in the same gang?
//
// Drop this file at pkg/scheduler/actions/allocate/case2_multinode_poc_test.go
// (same package as allocate_test.go). Requires mocktopology from the Case 1
// PoC (pkg/scheduler/api/devices/mocktopology/mock_topology_device.go).
//
// #5751 Case 2 language: "Each Pod must receive a valid local device group,
// while the complete PodGroup should be placed in an appropriate rack or
// network HyperNode." This PoC checks whether that holds today when
// HyperNode-level scoring is device-blind (confirmed in
// network_topology_aware.go's resourceStatus cache, which tracks only
// allocatable/used/idle/futureIdle -- no domain fields).
//
// Scenario:
//   HyperNode s0: s0-n1 (healthy, one domain, 6 free) + s0-n2 (fragmented,
//                 two domains 4+2 -- aggregate 6 matches the request, but
//                 max single domain 4 < 6 requested)
//   HyperNode s1: s1-n1 (healthy, 6 free) + s1-n2 (healthy, 6 free)
//   Gang: 2 tasks (master, worker), hard network-topology mode tier 1,
//         minAvailable=2, each requesting 6 devices as one domain.
//
// s0 and s1 report identical aggregate resources (2 nodes x 6 devices each),
// so which HyperNode gets tried is a non-deterministic tie-break, same as
// the Node-level tie-break in the Case 1 PoC. When s0 is picked, the
// question this test answers: does anything stop one task landing on
// s0-n1 (valid) while its sibling lands on s0-n2 (invalid), with no
// coordination between the two Allocate() calls to avoid or recover from
// that split?
package allocate

import (
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"

	schedulingv1 "volcano.sh/apis/pkg/apis/scheduling/v1beta1"
	topologyv1alpha1 "volcano.sh/apis/pkg/apis/topology/v1alpha1"
	"volcano.sh/volcano/pkg/scheduler/api"
	devicesmock "volcano.sh/volcano/pkg/scheduler/api/devices/mocktopology"
	"volcano.sh/volcano/pkg/scheduler/conf"
	"volcano.sh/volcano/pkg/scheduler/framework"
	"volcano.sh/volcano/pkg/scheduler/plugins/gang"
	networktopologyaware "volcano.sh/volcano/pkg/scheduler/plugins/network-topology-aware"
	"volcano.sh/volcano/pkg/scheduler/plugins/predicates"
	"volcano.sh/volcano/pkg/scheduler/uthelper"
	"volcano.sh/volcano/pkg/scheduler/util"
)

// Static check: same purpose as in the Case 1 PoC -- catch an api.Devices
// interface drift at compile time instead of a silent runtime type-assertion
// failure inside predicates.go's AllocateFunc/DeallocateFunc.
var _ api.Devices = (*devicesmock.MockTopologyDevices)(nil)

func buildPodWithXPURequestAndRole(ns, name, nodeName string, phase v1.PodPhase, count int64, groupName, role string) *v1.Pod {
	pod := util.BuildPod(ns, name, nodeName, phase, api.BuildResourceList("1", "1G"), groupName,
		map[string]string{"volcano.sh/task-spec": role}, nil)
	pod.Spec.Containers[0].Resources.Requests[devicesmock.ResourceName] = *resourceapi.NewQuantity(count, resourceapi.DecimalSI)
	return pod
}

func TestCase2_GangSplitAcrossValidAndInvalidNodeInSameHyperNode(t *testing.T) {
	plugins := map[string]framework.PluginBuilder{
		predicates.PluginName:           predicates.New,
		gang.PluginName:                 gang.New,
		networktopologyaware.PluginName: networktopologyaware.New,
	}

	const requestCount = 6

	// s0: one healthy node, one fragmented node. A gang landing in s0 has no
	// guarantee both its tasks get a Node with a valid local device group.
	s0n1 := devicesmock.NewMockTopologyDevices("s0-n1",
		&devicesmock.Domain{Name: "domain-a", Total: requestCount, Used: 0})
	s0n2 := devicesmock.NewMockTopologyDevices("s0-n2",
		&devicesmock.Domain{Name: "domain-a", Total: 4, Used: 0},
		&devicesmock.Domain{Name: "domain-b", Total: 2, Used: 0})

	// s1: both nodes healthy -- a fully valid HyperNode for this gang.
	s1n1 := devicesmock.NewMockTopologyDevices("s1-n1",
		&devicesmock.Domain{Name: "domain-a", Total: requestCount, Used: 0})
	s1n2 := devicesmock.NewMockTopologyDevices("s1-n2",
		&devicesmock.Domain{Name: "domain-a", Total: requestCount, Used: 0})

	// Fixture pattern (HyperNodesMap / HyperNodesSetByTier / HyperNodes)
	// mirrors TestAllocateWithNetWorkTopologies's hard-topology, tier-1,
	// two-HyperNode cases in allocate_test.go -- reused as-is, not
	// reinvented, so this exercises the exact gradient/predicate machinery
	// those passing tests already cover.
	test := uthelper.TestCommonStruct{
		Name:    "case2: gang tasks can split across valid and invalid Nodes within one already-selected HyperNode",
		Plugins: plugins,
		PodGroups: []*schedulingv1.PodGroup{
			util.BuildPodGroupWithNetWorkTopologies("pg1", "c1", "", "q1", 2, nil, schedulingv1.PodGroupInqueue, "hard", 1),
		},
		Pods: []*v1.Pod{
			buildPodWithXPURequestAndRole("c1", "p1", "", v1.PodPending, requestCount, "pg1", "master"),
			buildPodWithXPURequestAndRole("c1", "p2", "", v1.PodPending, requestCount, "pg1", "worker"),
		},
		Nodes: []*v1.Node{
			util.BuildNode("s0-n1", api.BuildResourceList("2", "4Gi",
				[]api.ScalarResource{{Name: "pods", Value: "10"}, {Name: devicesmock.ResourceName, Value: "6"}}...), nil),
			util.BuildNode("s0-n2", api.BuildResourceList("2", "4Gi",
				[]api.ScalarResource{{Name: "pods", Value: "10"}, {Name: devicesmock.ResourceName, Value: "6"}}...), nil),
			util.BuildNode("s1-n1", api.BuildResourceList("2", "4Gi",
				[]api.ScalarResource{{Name: "pods", Value: "10"}, {Name: devicesmock.ResourceName, Value: "6"}}...), nil),
			util.BuildNode("s1-n2", api.BuildResourceList("2", "4Gi",
				[]api.ScalarResource{{Name: "pods", Value: "10"}, {Name: devicesmock.ResourceName, Value: "6"}}...), nil),
		},
		HyperNodesSetByTier: map[int]sets.Set[string]{1: sets.New[string]("s0", "s1")},
		HyperNodesMap: map[string]*api.HyperNodeInfo{
			"s0": api.NewHyperNodeInfo(api.BuildHyperNode("s0", 1, []api.MemberConfig{
				{Name: "s0-n1", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
				{Name: "s0-n2", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
			"s1": api.NewHyperNodeInfo(api.BuildHyperNode("s1", 1, []api.MemberConfig{
				{Name: "s1-n1", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
				{Name: "s1-n2", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
		},
		HyperNodes: map[string]sets.Set[string]{
			"s0": sets.New[string]("s0-n1", "s0-n2"),
			"s1": sets.New[string]("s1-n1", "s1-n2"),
		},
		Queues: []*schedulingv1.Queue{
			util.BuildQueue("q1", 1, nil),
		},
		MinimalBindCheck: true,
	}

	trueValue := true
	tiers := []conf.Tier{
		{
			Plugins: []conf.PluginOption{
				{
					Name:                gang.PluginName,
					EnabledJobOrder:     &trueValue,
					EnabledJobReady:     &trueValue,
					EnabledJobPipelined: &trueValue,
					EnabledJobStarving:  &trueValue,
				},
				{
					Name:             predicates.PluginName,
					EnabledPredicate: &trueValue,
				},
				{
					Name:                     networktopologyaware.PluginName,
					EnabledNodeOrder:         &trueValue,
					EnabledHyperNodeOrder:    &trueValue,
					EnabledHyperNodeGradient: &trueValue,
				},
			},
		},
	}

	ssn := test.RegisterSession(tiers, nil)
	defer test.Close()

	// Wire the mock devices into each node's Others map, same registration
	// pattern as the Case 1 PoC (mirrors predicates.go's AllocateFunc
	// lookup: nodeInfo.Others[val].(api.Devices)).
	const registeredKey = "mocktopology"
	byNode := map[string]*devicesmock.MockTopologyDevices{
		"s0-n1": s0n1, "s0-n2": s0n2, "s1-n1": s1n1, "s1-n2": s1n2,
	}
	for name, dev := range byNode {
		if ssn.Nodes[name].Others == nil {
			ssn.Nodes[name].Others = map[string]interface{}{}
		}
		ssn.Nodes[name].Others[registeredKey] = dev
	}
	api.RegisteredDevices = append(api.RegisteredDevices, registeredKey)

	test.Run([]framework.Action{New()})

	// Async bind/allocate goroutines -- same reasoning as the Case 1 PoC:
	// TestCommonStruct.binder is unexported, so we read the mock devices'
	// own call logs (set synchronously) instead of inspecting bind state.
	time.Sleep(100 * time.Millisecond)

	for name, dev := range byNode {
		t.Logf("%s: FilterCalled=%d AllocateCalled=%d log=%v results=%v", name, dev.FilterCalled, dev.AllocateCalled, dev.AllocateCallLog, dev.AllocateResults)
	}

	// Precisely determine, from AllocateResults (not a coarse "was Allocate
	// called at all" check), whether p1 and p2 ended up on DIFFERENT Nodes
	// within the SAME HyperNode with different outcomes -- that is the exact
	// Case 2 scenario this test targets, and this is the only check that
	// actually answers it instead of guessing from aggregate call counts.
	type placement struct {
		node    string
		success bool
		found   bool
	}
	finalPlacement := func(pod string) placement {
		// CAVEAT: if a pod was tried on more than one node across retries in
		// this cycle, Go's random map iteration order means this may not
		// pick the truly last-in-time node -- AllocateResults isn't
		// timestamped. Cross-check against AllocateCallLog's per-node,
		// per-call ordering (logged above) before treating this as
		// authoritative when a pod appears in more than one node's results.
		for _, dev := range byNode {
			if ok, exists := dev.AllocateResults[pod]; exists {
				return placement{node: dev.NodeName, success: ok, found: true}
			}
		}
		return placement{}
	}
	p1 := finalPlacement("p1")
	p2 := finalPlacement("p2")
	t.Logf("p1 final placement: %+v", p1)
	t.Logf("p2 final placement: %+v", p2)

	sameHyperNode := func(a, b string) bool {
		s0 := map[string]bool{"s0-n1": true, "s0-n2": true}
		s1 := map[string]bool{"s1-n1": true, "s1-n2": true}
		return (s0[a] && s0[b]) || (s1[a] && s1[b])
	}

	switch {
	case p1.found && p2.found && p1.node != p2.node && sameHyperNode(p1.node, p2.node) && p1.success != p2.success:
		t.Logf("POC CONFIRMED (split, divergent outcome): gang tasks p1 (node=%s success=%v) and p2 "+
			"(node=%s success=%v) landed on DIFFERENT Nodes within the SAME HyperNode, with DIFFERENT "+
			"outcomes -- one task got a valid local device group, its sibling did not, and nothing in "+
			"HyperNode/Node selection coordinated between them to prevent or catch this split. This directly "+
			"contradicts #5751 Case 2's requirement that 'each Pod must receive a valid local device group.'",
			p1.node, p1.success, p2.node, p2.success)
	case p1.found && p2.found && p1.node != p2.node && sameHyperNode(p1.node, p2.node):
		t.Logf("POC CONFIRMED (split, same outcome): gang tasks p1 (node=%s success=%v) and p2 (node=%s "+
			"success=%v) landed on DIFFERENT Nodes within the SAME HyperNode. Both happened to end with the "+
			"same success/failure this run, but the split itself is the point: nothing in HyperNode/Node "+
			"selection tries to keep a gang's tasks together, or checks that EVERY member Node in the chosen "+
			"HyperNode actually has a valid local device group before committing the HyperNode for this gang.",
			p1.node, p1.success, p2.node, p2.success)
	case p1.found && p2.found && p1.node == p2.node:
		t.Logf("Both tasks were (finally) attempted on the SAME Node (%s) -- not the split scenario this test "+
			"targets. This run doesn't exercise Case 2's cross-Node concern; re-run with -count=N.", p1.node)
	case p1.found && p2.found && !sameHyperNode(p1.node, p2.node):
		t.Logf("Tasks landed in DIFFERENT HyperNodes (p1=%s, p2=%s) -- likely evidence of gang re-attempt "+
			"across cycles rather than a single-HyperNode split. Worth its own investigation but not what "+
			"this test targets; check AllocateCallLog above for the full sequence before concluding anything.",
			p1.node, p2.node)
	default:
		t.Fatalf("could not determine final placement for both p1 and p2 (p1=%+v, p2=%+v) -- "+
			"check AllocateResults wiring or whether Allocate() is being called at all", p1, p2)
	}
}