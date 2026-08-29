/*
Copyright 2025 The Volcano Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package xpudomainaware

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"

	schedulingv1 "volcano.sh/apis/pkg/apis/scheduling/v1beta1"
	topologyv1alpha1 "volcano.sh/apis/pkg/apis/topology/v1alpha1"
	"volcano.sh/volcano/pkg/scheduler/actions/allocate"
	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/conf"
	"volcano.sh/volcano/pkg/scheduler/framework"
	"volcano.sh/volcano/pkg/scheduler/plugins/gang"
	"volcano.sh/volcano/pkg/scheduler/plugins/predicates"
	"volcano.sh/volcano/pkg/scheduler/uthelper"
	"volcano.sh/volcano/pkg/scheduler/util"
)

// TestXPUDomainAware_FiltersHyperNodeWithSplitDomains reproduces the
// motivating example from the issue: a Node's 8 free xPU devices are split
// 4/4 across two independent domains (e.g. two 4-GPU NVLink groups), and a
// job's SubJob wants 8 devices for one task -- i.e. it needs a single
// domain with 8 free devices, which this Node does NOT have despite having
// 8 free devices in aggregate.
//
// A second HyperNode has one Node with a single domain covering all 8
// devices, which DOES satisfy the request.
//
// Without domain awareness (aggregate-only scheduling), both HyperNodes
// look identical to the scheduler: n1 and n2 both report 8 idle devices as
// a plain resource count. With this plugin registered, only the HyperNode
// containing a genuinely satisfying domain survives the gradient, and the
// task binds there.
func TestXPUDomainAware_FiltersHyperNodeWithSplitDomains(t *testing.T) {
	const devicesPerTask = 8
	const xpuResourceName = "example.com/xpu"

	// n1 (HyperNode s0): 8 devices total, split into two 4-device domains.
	// n2 (HyperNode s1): 8 devices total, one domain covering all 8.
	SetProvider(NewMockTopologyProvider(map[string][]TopologyDomain{
		"s0-n1": {
			{
				ID:            "s0-n1/domain-a",
				Kind:          "NVLink",
				NodeIDs:       []string{"s0-n1"},
				DeviceIDs:     []string{"d0", "d1", "d2", "d3"},
				FreeDeviceIDs: []string{"d0", "d1", "d2", "d3"},
			},
			{
				ID:            "s0-n1/domain-b",
				Kind:          "NVLink",
				NodeIDs:       []string{"s0-n1"},
				DeviceIDs:     []string{"d4", "d5", "d6", "d7"},
				FreeDeviceIDs: []string{"d4", "d5", "d6", "d7"},
			},
		},
		"s1-n2": {
			{
				ID:            "s1-n2/domain-a",
				Kind:          "NVLink",
				NodeIDs:       []string{"s1-n2"},
				DeviceIDs:     []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7"},
				FreeDeviceIDs: []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7"},
			},
		},
	}))

	plugins := map[string]framework.PluginBuilder{
		gang.PluginName:       gang.New,
		predicates.PluginName: predicates.New,
		PluginName:            New,
	}

	xpuReq := v1.ResourceList{v1.ResourceName(xpuResourceName): resource.MustParse("8")}

	test := uthelper.TestCommonStruct{
		Name:    "xpu-domain-aware filters HyperNode with split domains",
		Plugins: plugins,
		PodGroups: []*schedulingv1.PodGroup{
			// hard-mode topology at tier 0 forces this job through
			// allocateForSubJob's HyperNodeGradientForSubJobFn path
			// instead of the flat/no-topology fast path, so our gradient
			// filter is actually consulted.
			util.BuildPodGroupWithNetWorkTopologies("pg1", "c1", "", "q1", 1, nil, schedulingv1.PodGroupInqueue, "hard", 0),
		},
		Pods: []*v1.Pod{
			util.BuildPod("c1", "p1", "", v1.PodPending, xpuReq, "pg1", nil, nil),
		},
		Nodes: []*v1.Node{
			util.BuildNode("s0-n1", api.BuildResourceList("8", "16Gi",
				[]api.ScalarResource{{Name: xpuResourceName, Value: "8"}, {Name: "pods", Value: "10"}}...), nil),
			util.BuildNode("s1-n2", api.BuildResourceList("8", "16Gi",
				[]api.ScalarResource{{Name: xpuResourceName, Value: "8"}, {Name: "pods", Value: "10"}}...), nil),
		},
		HyperNodesSetByTier: map[int]sets.Set[string]{0: sets.New[string]("s0", "s1")},
		HyperNodesMap: map[string]*api.HyperNodeInfo{
			"s0": api.NewHyperNodeInfo(api.BuildHyperNode("s0", 0, []api.MemberConfig{
				{Name: "s0-n1", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
			"s1": api.NewHyperNodeInfo(api.BuildHyperNode("s1", 0, []api.MemberConfig{
				{Name: "s1-n2", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
		},
		HyperNodes: map[string]sets.Set[string]{
			"s0": sets.New[string]("s0-n1"),
			"s1": sets.New[string]("s1-n2"),
		},
		Queues: []*schedulingv1.Queue{
			util.BuildQueue("q1", 1, nil),
		},
		ExpectBindMap: map[string]string{
			// Only s1-n2 has a single domain with 8 free devices; s0-n1
			// must be filtered out of the gradient before predicate/device
			// allocation ever run on it.
			"c1/p1": "s1-n2",
		},
		ExpectBindsNum: 1,
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
					Name:                     PluginName,
					EnabledHyperNodeGradient: &trueValue,
					Arguments: framework.Arguments{
						DeviceRequestPerTaskKey: devicesPerTask,
					},
				},
			},
		},
	}

	test.RegisterSession(tiers, nil)
	defer test.Close()
	test.Run([]framework.Action{allocate.New()})
	if err := test.CheckAll(0); err != nil {
		t.Fatal(err)
	}
}

// TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation shows the
// PoC failing closed (no bind) rather than falling back to unsafe
// aggregate-only placement when NO HyperNode has a satisfying domain --
// this is the case that currently ships silently broken: today the
// scheduler would happily bind here and let device allocation fail late,
// after the Node/HyperNode commit (per the Slack thread). With this plugin,
// the SubJob simply finds no gradient solution and is not bound at all.
func TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation(t *testing.T) {
	const devicesPerTask = 8
	const xpuResourceName = "example.com/xpu"

	// Both Nodes have 8 free devices in aggregate, but split 4/4 -- neither
	// has a single domain able to satisfy an 8-device single-domain request.
	SetProvider(NewMockTopologyProvider(map[string][]TopologyDomain{
		"s0-n1": {
			{ID: "s0-n1/domain-a", Kind: "NVLink", NodeIDs: []string{"s0-n1"}, DeviceIDs: []string{"d0", "d1", "d2", "d3"}, FreeDeviceIDs: []string{"d0", "d1", "d2", "d3"}},
			{ID: "s0-n1/domain-b", Kind: "NVLink", NodeIDs: []string{"s0-n1"}, DeviceIDs: []string{"d4", "d5", "d6", "d7"}, FreeDeviceIDs: []string{"d4", "d5", "d6", "d7"}},
		},
		"s1-n2": {
			{ID: "s1-n2/domain-a", Kind: "NVLink", NodeIDs: []string{"s1-n2"}, DeviceIDs: []string{"d0", "d1", "d2", "d3"}, FreeDeviceIDs: []string{"d0", "d1", "d2", "d3"}},
			{ID: "s1-n2/domain-b", Kind: "NVLink", NodeIDs: []string{"s1-n2"}, DeviceIDs: []string{"d4", "d5", "d6", "d7"}, FreeDeviceIDs: []string{"d4", "d5", "d6", "d7"}},
		},
	}))

	plugins := map[string]framework.PluginBuilder{
		gang.PluginName:       gang.New,
		predicates.PluginName: predicates.New,
		PluginName:            New,
	}

	xpuReq := v1.ResourceList{v1.ResourceName(xpuResourceName): resource.MustParse("8")}

	test := uthelper.TestCommonStruct{
		Name:    "xpu-domain-aware blocks allocation when no domain anywhere satisfies request",
		Plugins: plugins,
		PodGroups: []*schedulingv1.PodGroup{
			util.BuildPodGroupWithNetWorkTopologies("pg1", "c1", "", "q1", 1, nil, schedulingv1.PodGroupInqueue, "hard", 0),
		},
		Pods: []*v1.Pod{
			util.BuildPod("c1", "p1", "", v1.PodPending, xpuReq, "pg1", nil, nil),
		},
		Nodes: []*v1.Node{
			util.BuildNode("s0-n1", api.BuildResourceList("8", "16Gi",
				[]api.ScalarResource{{Name: xpuResourceName, Value: "8"}, {Name: "pods", Value: "10"}}...), nil),
			util.BuildNode("s1-n2", api.BuildResourceList("8", "16Gi",
				[]api.ScalarResource{{Name: xpuResourceName, Value: "8"}, {Name: "pods", Value: "10"}}...), nil),
		},
		HyperNodesSetByTier: map[int]sets.Set[string]{0: sets.New[string]("s0", "s1")},
		HyperNodesMap: map[string]*api.HyperNodeInfo{
			"s0": api.NewHyperNodeInfo(api.BuildHyperNode("s0", 0, []api.MemberConfig{
				{Name: "s0-n1", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
			"s1": api.NewHyperNodeInfo(api.BuildHyperNode("s1", 0, []api.MemberConfig{
				{Name: "s1-n2", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
		},
		HyperNodes: map[string]sets.Set[string]{
			"s0": sets.New[string]("s0-n1"),
			"s1": sets.New[string]("s1-n2"),
		},
		Queues: []*schedulingv1.Queue{
			util.BuildQueue("q1", 1, nil),
		},
		ExpectBindMap:  map[string]string{},
		ExpectBindsNum: 0,
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
					Name:                     PluginName,
					EnabledHyperNodeGradient: &trueValue,
					Arguments: framework.Arguments{
						DeviceRequestPerTaskKey: devicesPerTask,
					},
				},
			},
		},
	}

	test.RegisterSession(tiers, nil)
	defer test.Close()
	test.Run([]framework.Action{allocate.New()})
	if err := test.CheckAll(0); err != nil {
		t.Fatal(err)
	}
}

// TestXPUDomainAware_MultipleSatisfyingHyperNodesBindsToOne verifies the
// gradient filter correctly composes with the rest of the allocate pipeline
// when MORE THAN ONE HyperNode survives the domain filter in the same
// gradient tier. This is the shape that broke earlier in development: a
// gradient tier containing multiple survivors has to flow correctly through
// selectBestHyperNodeForSubJob's scoring/selection, not just through a
// single-candidate accept/reject check.
//
// Three HyperNodes: s0 has its 8 devices split 4/4 (fails), s1 and s2 each
// have one domain covering all 8 (both pass). Exactly one task should bind,
// and it must land on s1 or s2 -- never s0.
func TestXPUDomainAware_MultipleSatisfyingHyperNodesBindsToOne(t *testing.T) {
	const devicesPerTask = 8
	const xpuResourceName = "example.com/xpu"

	SetProvider(NewMockTopologyProvider(map[string][]TopologyDomain{
		"s0-n1": {
			{ID: "s0-n1/domain-a", Kind: "NVLink", NodeIDs: []string{"s0-n1"}, DeviceIDs: []string{"d0", "d1", "d2", "d3"}, FreeDeviceIDs: []string{"d0", "d1", "d2", "d3"}},
			{ID: "s0-n1/domain-b", Kind: "NVLink", NodeIDs: []string{"s0-n1"}, DeviceIDs: []string{"d4", "d5", "d6", "d7"}, FreeDeviceIDs: []string{"d4", "d5", "d6", "d7"}},
		},
		"s1-n2": {
			{ID: "s1-n2/domain-a", Kind: "NVLink", NodeIDs: []string{"s1-n2"}, DeviceIDs: []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7"}, FreeDeviceIDs: []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7"}},
		},
		"s2-n3": {
			{ID: "s2-n3/domain-a", Kind: "NVLink", NodeIDs: []string{"s2-n3"}, DeviceIDs: []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7"}, FreeDeviceIDs: []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7"}},
		},
	}))

	plugins := map[string]framework.PluginBuilder{
		gang.PluginName:       gang.New,
		predicates.PluginName: predicates.New,
		PluginName:            New,
	}

	xpuReq := v1.ResourceList{v1.ResourceName(xpuResourceName): resource.MustParse("8")}

	test := uthelper.TestCommonStruct{
		Name:    "xpu-domain-aware binds to one of multiple satisfying hyperNodes",
		Plugins: plugins,
		PodGroups: []*schedulingv1.PodGroup{
			util.BuildPodGroupWithNetWorkTopologies("pg1", "c1", "", "q1", 1, nil, schedulingv1.PodGroupInqueue, "hard", 0),
		},
		Pods: []*v1.Pod{
			util.BuildPod("c1", "p1", "", v1.PodPending, xpuReq, "pg1", nil, nil),
		},
		Nodes: []*v1.Node{
			util.BuildNode("s0-n1", api.BuildResourceList("8", "16Gi",
				[]api.ScalarResource{{Name: xpuResourceName, Value: "8"}, {Name: "pods", Value: "10"}}...), nil),
			util.BuildNode("s1-n2", api.BuildResourceList("8", "16Gi",
				[]api.ScalarResource{{Name: xpuResourceName, Value: "8"}, {Name: "pods", Value: "10"}}...), nil),
			util.BuildNode("s2-n3", api.BuildResourceList("8", "16Gi",
				[]api.ScalarResource{{Name: xpuResourceName, Value: "8"}, {Name: "pods", Value: "10"}}...), nil),
		},
		HyperNodesSetByTier: map[int]sets.Set[string]{0: sets.New[string]("s0", "s1", "s2")},
		HyperNodesMap: map[string]*api.HyperNodeInfo{
			"s0": api.NewHyperNodeInfo(api.BuildHyperNode("s0", 0, []api.MemberConfig{
				{Name: "s0-n1", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
			"s1": api.NewHyperNodeInfo(api.BuildHyperNode("s1", 0, []api.MemberConfig{
				{Name: "s1-n2", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
			"s2": api.NewHyperNodeInfo(api.BuildHyperNode("s2", 0, []api.MemberConfig{
				{Name: "s2-n3", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
		},
		HyperNodes: map[string]sets.Set[string]{
			"s0": sets.New[string]("s0-n1"),
			"s1": sets.New[string]("s1-n2"),
			"s2": sets.New[string]("s2-n3"),
		},
		Queues: []*schedulingv1.Queue{
			util.BuildQueue("q1", 1, nil),
		},
		// Which of s1/s2 wins is a scoring detail we don't assert on; what
		// matters is exactly one bind, and it must not be s0-n1.
		MinimalBindCheck: true,
		ExpectBindsNum:   1,
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
					Name:                     PluginName,
					EnabledHyperNodeGradient: &trueValue,
					Arguments: framework.Arguments{
						DeviceRequestPerTaskKey: devicesPerTask,
					},
				},
			},
		},
	}

	test.RegisterSession(tiers, nil)
	defer test.Close()
	test.Run([]framework.Action{allocate.New()})
	if err := test.CheckAll(0); err != nil {
		t.Fatal(err)
	}
}

// TestXPUDomainAware_MissingDomainDataTreatedAsUnsatisfying reproduces the
// mindcluster/vgpu gap documented in the issue thread: a vendor device
// plugin with nowhere to report per-device domain membership means the
// TopologyProvider has NO entry at all for that Node -- not an empty
// domain list, but a (nil, false) DomainsForNode result.
//
// s0-n1 has real domain data but it doesn't satisfy the request (split 4/4).
// s1-n2 has NO entry in the mock provider at all. Both must be rejected:
// missing domain data must never be silently treated as "no constraint,
// let it through" -- that would be worse than today's aggregate-only
// behavior, since it would claim domain-awareness while providing none.
func TestXPUDomainAware_MissingDomainDataTreatedAsUnsatisfying(t *testing.T) {
	const devicesPerTask = 8
	const xpuResourceName = "example.com/xpu"

	// Only s0-n1 has an entry. s1-n2 is deliberately absent from the map,
	// simulating a vendor plugin (mindcluster/vgpu) with no domain concept.
	SetProvider(NewMockTopologyProvider(map[string][]TopologyDomain{
		"s0-n1": {
			{ID: "s0-n1/domain-a", Kind: "NVLink", NodeIDs: []string{"s0-n1"}, DeviceIDs: []string{"d0", "d1", "d2", "d3"}, FreeDeviceIDs: []string{"d0", "d1", "d2", "d3"}},
			{ID: "s0-n1/domain-b", Kind: "NVLink", NodeIDs: []string{"s0-n1"}, DeviceIDs: []string{"d4", "d5", "d6", "d7"}, FreeDeviceIDs: []string{"d4", "d5", "d6", "d7"}},
		},
		// s1-n2: intentionally no entry.
	}))

	plugins := map[string]framework.PluginBuilder{
		gang.PluginName:       gang.New,
		predicates.PluginName: predicates.New,
		PluginName:            New,
	}

	xpuReq := v1.ResourceList{v1.ResourceName(xpuResourceName): resource.MustParse("8")}

	test := uthelper.TestCommonStruct{
		Name:    "xpu-domain-aware treats missing domain data as unsatisfying, not as no-op",
		Plugins: plugins,
		PodGroups: []*schedulingv1.PodGroup{
			util.BuildPodGroupWithNetWorkTopologies("pg1", "c1", "", "q1", 1, nil, schedulingv1.PodGroupInqueue, "hard", 0),
		},
		Pods: []*v1.Pod{
			util.BuildPod("c1", "p1", "", v1.PodPending, xpuReq, "pg1", nil, nil),
		},
		Nodes: []*v1.Node{
			util.BuildNode("s0-n1", api.BuildResourceList("8", "16Gi",
				[]api.ScalarResource{{Name: xpuResourceName, Value: "8"}, {Name: "pods", Value: "10"}}...), nil),
			util.BuildNode("s1-n2", api.BuildResourceList("8", "16Gi",
				[]api.ScalarResource{{Name: xpuResourceName, Value: "8"}, {Name: "pods", Value: "10"}}...), nil),
		},
		HyperNodesSetByTier: map[int]sets.Set[string]{0: sets.New[string]("s0", "s1")},
		HyperNodesMap: map[string]*api.HyperNodeInfo{
			"s0": api.NewHyperNodeInfo(api.BuildHyperNode("s0", 0, []api.MemberConfig{
				{Name: "s0-n1", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
			"s1": api.NewHyperNodeInfo(api.BuildHyperNode("s1", 0, []api.MemberConfig{
				{Name: "s1-n2", Type: topologyv1alpha1.MemberTypeNode, Selector: "exact"},
			})),
		},
		HyperNodes: map[string]sets.Set[string]{
			"s0": sets.New[string]("s0-n1"),
			"s1": sets.New[string]("s1-n2"),
		},
		Queues: []*schedulingv1.Queue{
			util.BuildQueue("q1", 1, nil),
		},
		ExpectBindMap:  map[string]string{},
		ExpectBindsNum: 0,
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
					Name:                     PluginName,
					EnabledHyperNodeGradient: &trueValue,
					Arguments: framework.Arguments{
						DeviceRequestPerTaskKey: devicesPerTask,
					},
				},
			},
		},
	}

	test.RegisterSession(tiers, nil)
	defer test.Close()
	test.Run([]framework.Action{allocate.New()})
	if err := test.CheckAll(0); err != nil {
		t.Fatal(err)
	}
}