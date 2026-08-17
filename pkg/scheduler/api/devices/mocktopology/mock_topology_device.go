// Package mocktopology is a PoC-only fake Devices implementation used to prove
// that Volcano's allocate action commits to a Node/HyperNode *before* any
// device-domain check happens, and that a failed domain-aware Allocate() has
// no path back to try a different Node in the same HyperNode.
//
// Drop this file under pkg/scheduler/api/devices/mocktopology/ in your fork.
// It implements the api.Devices interface the same way ascend/nvidia do.
package mocktopology

import (
	"fmt"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"volcano.sh/volcano/pkg/scheduler/api/devices"
)

const (
	// ResourceName is the fake scalar resource requested by pods in the PoC.
	ResourceName = "poc.example.com/xpu"
	// DomainAnnotation records the assigned domain, used by the test to assert.
	DomainAnnotation = "poc.example.com/xpu-domain"
)

// Domain models one physical interconnect domain (an HCCS/NVLink group) on a node.
type Domain struct {
	Name     string
	Total    int
	Used     int
}

func (d *Domain) Free() int { return d.Total - d.Used }

// MockTopologyDevices is intentionally simple: a fixed node with N domains,
// each with its own free-device count. This is what lets us reproduce the
// exact "6 free in one domain, 2 free in another, 8 requested" scenario from
// the issue, entirely inside a unit test.
type MockTopologyDevices struct {
	mu       sync.Mutex
	NodeName string
	Domains  []*Domain

	// FilterCalled / AllocateCalled let the test assert call *order* and
	// *count* -- the actual thing we're trying to prove.
	FilterCalled   int
	AllocateCalled int
	// AllocateCallLog records one line per Allocate() call: the pod name and
	// the aggregate-free count *as seen by that call*. This exists because
	// the first PoC run showed Volcano calling Allocate() twice for a single
	// task in a single cycle, which needed distinguishing from "two different
	// nodes tried" -- the log makes that visible without guessing.
	AllocateCallLog []string
}

func NewMockTopologyDevices(nodeName string, domains ...*Domain) *MockTopologyDevices {
	return &MockTopologyDevices{NodeName: nodeName, Domains: domains}
}

func (m *MockTopologyDevices) requestedCount(pod *v1.Pod) int {
	total := 0
	for _, c := range pod.Spec.Containers {
		if q, ok := c.Resources.Requests[ResourceName]; ok {
			total += int(q.Value())
		}
	}
	return total
}

func (m *MockTopologyDevices) HasDeviceRequest(pod *v1.Pod) bool {
	return m.requestedCount(pod) > 0
}

// AddResource / SubResource are called from node_info.go's AddPod/RemovePod
// path to keep the device pool's internal ledger consistent with the node's
// actual pod set (separate from Allocate/Release, which run during the
// predicate/allocate cycle). For this PoC we don't need them to do anything
// beyond satisfy the interface -- domain accounting is driven entirely
// through Allocate/Release, which is what the allocate action actually calls.
func (m *MockTopologyDevices) AddResource(pod *v1.Pod) {}
func (m *MockTopologyDevices) SubResource(pod *v1.Pod) {}

// aggregateFree is what a naive/aggregate-only check would see -- this is
// deliberately what predicates.go / nodeorder / network-topology-aware see
// today, since none of them call into domain-aware Devices logic pre-bind.
func (m *MockTopologyDevices) aggregateFree() int {
	total := 0
	for _, d := range m.Domains {
		total += d.Free()
	}
	return total
}

// domainThatFits returns the first domain able to satisfy the request as one
// contiguous group, or nil if no single domain can.
func (m *MockTopologyDevices) domainThatFits(n int) *Domain {
	for _, d := range m.Domains {
		if d.Free() >= n {
			return d
		}
	}
	return nil
}

// FilterNode intentionally mimics today's real predicate plugins: it is NOT
// wired into the main Predicate() path (see predicates.go's Filter loop,
// which never calls Devices.FilterNode -- only AddSimulatePredicateFn does,
// and that's dry-run-only). We still implement it faithfully so the PoC can
// show that even a *correct* FilterNode implementation is not consulted on
// the real allocate path.
func (m *MockTopologyDevices) FilterNode(pod *v1.Pod, schedulePolicy string) (int, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FilterCalled++

	req := m.requestedCount(pod)
	if req == 0 {
		return devices.Success, "", nil
	}
	if m.domainThatFits(req) == nil {
		return devices.Unschedulable, fmt.Sprintf(
			"no single domain on node %s has %d free devices (aggregate free=%d, fragmented)",
			m.NodeName, req, m.aggregateFree()), nil
	}
	return devices.Success, "", nil
}

func (m *MockTopologyDevices) ScoreNode(pod *v1.Pod, schedulePolicy string) float64 { return 0 }

// Allocate is where the real domain check happens today -- AFTER Node/HyperNode
// commitment, inside predicates.go's AllocateFunc event handler. If it fails
// here, the task is skipped (allocateResourcesForTask logs+continues) but the
// Node has already been chosen for this scheduling attempt and is not retried
// against a *different* node within the same cycle.
//
// IMPORTANT (found via the actual PoC run): Allocate() must NOT mutate any
// state before it's certain the allocation will succeed. Volcano's Statement
// rollback path (see statement.go) treats a returned error as "nothing was
// mutated" and does not call Release() to undo a failed Allocate() -- so if
// this method mutates domain state before checking feasibility, a second
// attempt on the SAME node in the same cycle will see already-consumed state
// from the first failed call, which is a misleading artifact, not evidence
// of cross-node retry. Compute-then-commit avoids that: check feasibility
// fully, and only mutate on the success path.
func (m *MockTopologyDevices) Allocate(kubeClient kubernetes.Interface, pod *v1.Pod) (*devices.DeviceReservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AllocateCalled++
	m.AllocateCallLog = append(m.AllocateCallLog, fmt.Sprintf(
		"call#%d pod=%s aggregateFreeAtCallTime=%d", m.AllocateCalled, pod.Name, m.aggregateFree()))

	req := m.requestedCount(pod)
	d := m.domainThatFits(req)
	if d == nil {
		// This is the failure the issue describes: aggregate free (8) >= req (8),
		// but no *domain* has enough. This error surfaces only now, post-commit.
		// Nothing is mutated on this path -- the check happens before any write.
		return nil, fmt.Errorf(
			"POC: node %s has %d aggregate free devices (enough) but no single domain "+
				"has %d free (fragmented across %d domains) -- allocation fails after Node/HyperNode already chosen",
			m.NodeName, m.aggregateFree(), req, len(m.Domains))
	}
	d.Used += req
	return &devices.DeviceReservation{
		DeviceType:  "mocktopology",
		Annotations: map[string]string{DomainAnnotation: d.Name},
	}, nil
}

func (m *MockTopologyDevices) Release(kubeClient kubernetes.Interface, pod *v1.Pod) (*devices.DeviceReservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	req := m.requestedCount(pod)
	// PoC simplification: release from first domain with used > 0.
	for _, d := range m.Domains {
		if d.Used >= req {
			d.Used -= req
			break
		}
	}
	return &devices.DeviceReservation{DeviceType: "mocktopology"}, nil
}

func (m *MockTopologyDevices) GetStatus() string       { return "" }
func (m *MockTopologyDevices) GetIgnoredDevices() []string { return nil }
func (m *MockTopologyDevices) AddQueueResource(pod *v1.Pod) map[string]float64 { return nil }
func (m *MockTopologyDevices) DeepCopy() interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := &MockTopologyDevices{NodeName: m.NodeName}
	for _, d := range m.Domains {
		cp.Domains = append(cp.Domains, &Domain{Name: d.Name, Total: d.Total, Used: d.Used})
	}
	return cp
}