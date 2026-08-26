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
	"k8s.io/klog/v2"

	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/framework"
)

const (
	// PluginName indicates name of this scheduler plugin.
	PluginName = "xpu-domain-aware"

	// DeviceRequestPerTaskKey is the plugin argument giving the number of
	// xPU devices each task in the SubJob requests that must land in a
	// single domain together. In the full project this would instead be
	// read off the task's resource requests / DRA claim; a plugin argument
	// keeps this PoC self-contained and easy to demo.
	DeviceRequestPerTaskKey = "xpu.domain.devices-per-task"

	// DefaultDevicesPerTask is used if the argument above is not set.
	DefaultDevicesPerTask = 1
)

// xpuDomainAwarePlugin filters HyperNode gradients -- the same mechanism
// network-topology-aware.go uses via AddHyperNodeGradientForSubJobFn -- down
// to HyperNodes that contain at least one Node with at least one device
// domain able to satisfy the SubJob's per-task device requirement in a
// single domain.
//
// This runs inside allocate.go's allocateForSubJob, BEFORE PredicateNodes or
// any device Allocate() call executes, closing exactly the gap described in
// the Slack thread: HyperNode selection today only sees resource
// aggregates; Node selection (predicates.go) has no device-domain
// awareness either; device allocation happens last and can fail after the
// Node/HyperNode are already committed. This plugin makes domain
// availability visible at the earliest of those three points.
type xpuDomainAwarePlugin struct {
	pluginArguments framework.Arguments
	provider        TopologyProvider
	devicesPerTask  int
}

// New returns the plugin. The TopologyProvider is expected to be swapped in
// by whoever wires this into a real scheduler config; for this PoC's own
// test/demo it defaults to a MockTopologyProvider if none is registered.
var globalProvider TopologyProvider = NewMockTopologyProvider(nil)

// SetProvider lets tests/demos install a specific mock topology before
// OnSessionOpen runs. A production version would instead have the scheduler
// cache own and inject the provider; this indirection is only here to keep
// the PoC's New(framework.Arguments) signature matching Volcano's
// framework.PluginBuilder without threading extra state through session
// config.
func SetProvider(p TopologyProvider) { globalProvider = p }

func New(arguments framework.Arguments) framework.Plugin {
	devicesPerTask := DefaultDevicesPerTask
	arguments.GetInt(&devicesPerTask, DeviceRequestPerTaskKey)
	if devicesPerTask <= 0 {
		devicesPerTask = DefaultDevicesPerTask
	}
	return &xpuDomainAwarePlugin{
		pluginArguments: arguments,
		provider:        globalProvider,
		devicesPerTask:  devicesPerTask,
	}
}

func (xp *xpuDomainAwarePlugin) Name() string {
	return PluginName
}

func (xp *xpuDomainAwarePlugin) OnSessionOpen(ssn *framework.Session) {
	klog.V(4).Infof("Enter xpuDomainAwarePlugin plugin, devicesPerTask=%d ...", xp.devicesPerTask)

	ssn.AddHyperNodeGradientForSubJobFn(xp.Name(), func(subJob *api.SubJobInfo, hyperNode *api.HyperNodeInfo, purpose api.SearchPurpose) [][]*api.HyperNodeInfo {
		// Only gate SubJobs that actually declare an xPU domain requirement;
		// everything else is untouched, exactly like network-topology-aware
		// no-ops for jobs without hard/soft topology mode.
		if xp.devicesPerTask <= 0 {
			return nil
		}

		// hyperNode here is the search root (typically ClusterTopHyperNode),
		// NOT an individual candidate -- HyperNodeGradientForSubJobFn's job
		// is to EXPAND it into the actual candidate HyperNodes and filter
		// them, mirroring network-topology-aware's hyperNodeGradientFn BFS.
		// A single accept/reject check against the root itself is wrong:
		// the root's own RealNodesList is the union of every leaf
		// HyperNode's nodes, so any one satisfying domain anywhere in the
		// cluster would incorrectly "accept" the whole root.
		var survivors []*api.HyperNodeInfo
		for name, hn := range ssn.HyperNodes {
			if name == framework.ClusterTopHyperNode {
				continue
			}
			if _, ok := ssn.RealNodesList[name]; !ok {
				continue
			}
			if xp.hyperNodeHasSatisfyingDomain(ssn, hn) {
				klog.V(4).InfoS("xpu-domain-aware: accepting candidate hyperNode",
					"subJob", subJob.UID, "hyperNode", name)
				survivors = append(survivors, hn)
			} else {
				klog.V(4).InfoS("xpu-domain-aware: rejecting candidate hyperNode, no domain with enough free devices",
					"subJob", subJob.UID, "hyperNode", name, "devicesPerTask", xp.devicesPerTask)
			}
		}

		if len(survivors) == 0 {
			return [][]*api.HyperNodeInfo{{}}
		}
		return [][]*api.HyperNodeInfo{survivors}
	})
}

// hyperNodeHasSatisfyingDomain returns true if at least one Node under
// hyperNode has at least one TopologyDomain with FreeCount() >=
// xp.devicesPerTask. This is the core "domain info before HyperNode/Node
// selection" check.
func (xp *xpuDomainAwarePlugin) hyperNodeHasSatisfyingDomain(ssn *framework.Session, hyperNode *api.HyperNodeInfo) bool {
	nodes, ok := ssn.RealNodesList[hyperNode.Name]
	if !ok || len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		domains, found := xp.provider.DomainsForNode(node.Name)
		if !found {
			continue
		}
		for _, d := range domains {
			if d.FreeCount() >= xp.devicesPerTask {
				return true
			}
		}
	}
	return false
}

func (xp *xpuDomainAwarePlugin) OnSessionClose(ssn *framework.Session) {}