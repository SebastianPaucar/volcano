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
	"fmt"
	"sort"
)

// GangDeviceRequest is one task's device requirement within a gang. TaskID
// only needs to be unique within a single PlanGangDeviceIDs call.
type GangDeviceRequest struct {
	TaskID         string
	DevicesPerTask int
}

// PlanGangDeviceIDs selects disjoint device IDs for every task in a gang
// from a single TopologyDomain that has already been confirmed to satisfy
// the domain-selection check (hyperNodeHasSatisfyingDomain / FreeCount).
//
// This is deliberately narrow: it answers Expected Outcome #2's "selects
// concrete device IDs" and "supports gang-level planning" for the ALREADY
// CHOSEN winning domain -- it does not repeat HyperNode/domain selection,
// and it does not touch reservation state (no Volcano AllocateFunc/
// DeAllocateFunc wiring; see README's out-of-scope list).
//
// It answers the Case 2 gap directly: today, once a HyperNode is chosen,
// nothing stops two tasks in the same gang from being handed overlapping
// devices. Aggregate free-count alone isn't sufficient proof of a valid
// gang placement -- a caller still needs disjoint per-task assignments,
// which this function produces deterministically.
//
// PlanGangDeviceIDs returns a complete, disjoint assignment for every
// request, or an error and NO partial assignment -- callers must not act on
// a partial result.
func PlanGangDeviceIDs(domain TopologyDomain, requests []GangDeviceRequest) (map[string][]string, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("xpu-domain-aware: no gang device requests given")
	}

	// Duplicate TaskID is a caller bug, not a placement failure -- fail
	// loudly rather than silently overwrite one task's assignment.
	seen := make(map[string]bool, len(requests))
	total := 0
	for _, r := range requests {
		if r.TaskID == "" {
			return nil, fmt.Errorf("xpu-domain-aware: gang device request has empty TaskID")
		}
		if seen[r.TaskID] {
			return nil, fmt.Errorf("xpu-domain-aware: duplicate TaskID %q in gang device request", r.TaskID)
		}
		seen[r.TaskID] = true
		if r.DevicesPerTask <= 0 {
			return nil, fmt.Errorf("xpu-domain-aware: task %q requested non-positive device count %d", r.TaskID, r.DevicesPerTask)
		}
		total += r.DevicesPerTask
	}

	// Fast rejection: even a perfect packing can't succeed if the domain
	// doesn't have enough free devices in aggregate.
	if total > domain.FreeCount() {
		return nil, fmt.Errorf("xpu-domain-aware: gang requests %d devices total, domain %q has only %d free",
			total, domain.ID, domain.FreeCount())
	}

	// Deterministic order: sort free device IDs once, then sort requests by
	// TaskID so repeated calls with the same input always produce the same
	// assignment -- same determinism property the competing PoC's
	// SelectLocalDomain establishes, useful for testing and for debugging
	// production behavior.
	free := append([]string(nil), domain.FreeDeviceIDs...)
	sort.Strings(free)

	orderedRequests := append([]GangDeviceRequest(nil), requests...)
	sort.Slice(orderedRequests, func(i, j int) bool {
		return orderedRequests[i].TaskID < orderedRequests[j].TaskID
	})

	assignment := make(map[string][]string, len(requests))
	cursor := 0
	for _, r := range orderedRequests {
		assignment[r.TaskID] = append([]string(nil), free[cursor:cursor+r.DevicesPerTask]...)
		cursor += r.DevicesPerTask
	}

	return assignment, nil
}