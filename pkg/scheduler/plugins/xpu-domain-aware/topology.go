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

// Package xpudomainaware is a minimal, standalone proof-of-concept for
// volcano-sh/volcano#5751 ("Generic xPU Topology-Aware Scheduling").
//
// SCOPE OF THIS POC:
//   - Defines a vendor-neutral TopologyDomain/TopologyProvider shape (Case 1:
//     intra-node device domains, e.g. two 8-GPU NVLink groups on one Node).
//   - Filters HyperNode gradients so that only HyperNodes containing at least
//     one single domain with enough free devices survive, BEFORE Node
//     selection/predicate/device-allocation run. This directly matches
//     Jesse Stutler's stated direction: "add some domain related info before
//     HyperNode/Node selection."
//
// EXPLICITLY OUT OF SCOPE for this PoC (left for the full project):
//   - Concrete device ID selection / AllocateFunc / DeAllocateFunc wiring.
//   - Reservation and gang-level rollback.
//   - Cross-node fabric domains (Case 3, e.g. GB200 NVL72).
//   - Real Device Plugin / DRA ResourceSlice ingestion (a mock provider is
//     used here, consistent with the project's own "mock topology data and
//     KWOK" allowance).
package xpudomainaware

import "fmt"

// TopologyDomain describes a set of devices that share a physical
// interconnect (HCCS domain, NVLink group, NVSwitch fabric, ...). NodeIDs
// has one entry for a node-local domain (Case 1); it is left generic so a
// later cross-node fabric domain (Case 3) can reuse the same struct.
type TopologyDomain struct {
	// ID uniquely identifies this domain, e.g. "s0-n1/domain-0".
	ID string
	// Kind is descriptive only: "HCCS", "NVLink", "NVSwitch", etc.
	Kind string
	// NodeIDs are the Volcano Node names this domain spans.
	NodeIDs []string
	// DeviceIDs are the device identifiers (vendor-specific, e.g. DeviceInfo.ID)
	// that belong to this domain.
	DeviceIDs []string
	// FreeDeviceIDs are the subset of DeviceIDs currently unallocated.
	FreeDeviceIDs []string
}

// FreeCount returns the number of currently-unallocated devices in the domain.
func (d TopologyDomain) FreeCount() int {
	return len(d.FreeDeviceIDs)
}

// TopologyProvider is the generic, source-agnostic interface the real
// project would implement against Node annotations, topology CRDs, Device
// Plugin companions, vendor APIs, or DRA ResourceSlices. Scheduling logic
// (see gradient.go) depends only on this interface, never on a concrete
// source, satisfying the "topology ingestion must be independent of
// scheduling policy" requirement.
type TopologyProvider interface {
	// DomainsForNode returns all device domains physically located on the
	// given Node. Returns (nil, false) if the provider has no domain data
	// for that Node (e.g. Node has no xPU devices, or the vendor plugin in
	// use has nowhere to report domain membership -- see mindcluster/vgpu
	// findings in issue #5751 Case 1).
	DomainsForNode(nodeName string) ([]TopologyDomain, bool)
	// Refresh re-syncs domain state from the underlying source. In this PoC
	// it's a no-op on the mock provider; a real provider would re-read
	// annotations/CRDs/ResourceSlices here and is expected to be called
	// incrementally by the scheduler cache, per the project's Expected
	// Outcome #2.
	Refresh() error
}

// MockTopologyProvider is a static, in-memory TopologyProvider, matching
// the project's explicit allowance to validate with "mock topology data and
// KWOK, without requiring physical NVL72 or other accelerator hardware."
type MockTopologyProvider struct {
	// nodeDomains maps Node name -> its device domains.
	nodeDomains map[string][]TopologyDomain
}

// NewMockTopologyProvider builds a provider from a caller-supplied
// Node -> domains map, so tests and demos can shape arbitrary domain
// topologies (single domain, split domains, insufficient-per-domain, etc.).
func NewMockTopologyProvider(nodeDomains map[string][]TopologyDomain) *MockTopologyProvider {
	return &MockTopologyProvider{nodeDomains: nodeDomains}
}

func (m *MockTopologyProvider) DomainsForNode(nodeName string) ([]TopologyDomain, bool) {
	domains, ok := m.nodeDomains[nodeName]
	return domains, ok
}

func (m *MockTopologyProvider) Refresh() error { return nil }

// String is a debug helper.
func (d TopologyDomain) String() string {
	return fmt.Sprintf("domain{id=%s kind=%s nodes=%v free=%d/%d}",
		d.ID, d.Kind, d.NodeIDs, len(d.FreeDeviceIDs), len(d.DeviceIDs))
}