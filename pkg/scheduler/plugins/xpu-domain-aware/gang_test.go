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
)

// TestPlanGangDeviceIDs_DisjointAssignment verifies two tasks in the same
// gang, both landing in the same domain, receive non-overlapping device
// IDs -- the core Case 2 gap: aggregate capacity alone doesn't prove a
// gang can actually be placed without device collisions.
func TestPlanGangDeviceIDs_DisjointAssignment(t *testing.T) {
	domain := TopologyDomain{
		ID:            "s1-n2/domain-a",
		Kind:          "NVLink",
		NodeIDs:       []string{"s1-n2"},
		DeviceIDs:     []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7"},
		FreeDeviceIDs: []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7"},
	}

	assignment, err := PlanGangDeviceIDs(domain, []GangDeviceRequest{
		{TaskID: "task-a", DevicesPerTask: 4},
		{TaskID: "task-b", DevicesPerTask: 4},
	})
	if err != nil {
		t.Fatalf("expected successful gang plan, got error: %v", err)
	}

	if len(assignment["task-a"]) != 4 || len(assignment["task-b"]) != 4 {
		t.Fatalf("expected 4 devices per task, got task-a=%v task-b=%v", assignment["task-a"], assignment["task-b"])
	}

	seen := make(map[string]string)
	for taskID, deviceIDs := range assignment {
		for _, d := range deviceIDs {
			if owner, exists := seen[d]; exists {
				t.Fatalf("device %s assigned to both %s and %s", d, owner, taskID)
			}
			seen[d] = taskID
		}
	}
	if len(seen) != 8 {
		t.Fatalf("expected all 8 devices assigned exactly once, got %d unique assignments", len(seen))
	}
}

// TestPlanGangDeviceIDs_InsufficientAggregateCapacityFails verifies the
// gang plan fails cleanly, with NO partial assignment, when the domain
// cannot fit every task's request -- callers must never act on a partial
// map from a failed call.
func TestPlanGangDeviceIDs_InsufficientAggregateCapacityFails(t *testing.T) {
	domain := TopologyDomain{
		ID:            "s0-n1/domain-a",
		Kind:          "NVLink",
		NodeIDs:       []string{"s0-n1"},
		DeviceIDs:     []string{"d0", "d1", "d2", "d3", "d4", "d5"},
		FreeDeviceIDs: []string{"d0", "d1", "d2", "d3", "d4", "d5"}, // only 6 free
	}

	assignment, err := PlanGangDeviceIDs(domain, []GangDeviceRequest{
		{TaskID: "task-a", DevicesPerTask: 4},
		{TaskID: "task-b", DevicesPerTask: 4}, // 4+4=8 > 6 free
	})
	if err == nil {
		t.Fatalf("expected gang plan to fail when domain has insufficient aggregate capacity, got assignment: %v", assignment)
	}
	if assignment != nil {
		t.Fatalf("expected nil assignment on failure, got: %v", assignment)
	}
}

// TestPlanGangDeviceIDs_DeterministicAcrossCalls verifies repeated calls
// with the same domain and requests produce the identical assignment --
// necessary for predictable scheduling behavior and for tests/debugging.
func TestPlanGangDeviceIDs_DeterministicAcrossCalls(t *testing.T) {
	domain := TopologyDomain{
		ID:            "s1-n2/domain-a",
		Kind:          "NVLink",
		NodeIDs:       []string{"s1-n2"},
		DeviceIDs:     []string{"d3", "d1", "d0", "d2"},
		FreeDeviceIDs: []string{"d3", "d1", "d0", "d2"},
	}
	requests := []GangDeviceRequest{
		{TaskID: "task-b", DevicesPerTask: 2},
		{TaskID: "task-a", DevicesPerTask: 2},
	}

	first, err := PlanGangDeviceIDs(domain, requests)
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	second, err := PlanGangDeviceIDs(domain, requests)
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}

	for _, taskID := range []string{"task-a", "task-b"} {
		if len(first[taskID]) != len(second[taskID]) {
			t.Fatalf("non-deterministic assignment length for %s: first=%v second=%v", taskID, first[taskID], second[taskID])
		}
		for i := range first[taskID] {
			if first[taskID][i] != second[taskID][i] {
				t.Fatalf("non-deterministic assignment for %s: first=%v second=%v", taskID, first[taskID], second[taskID])
			}
		}
	}
}

// TestPlanGangDeviceIDs_RejectsInvalidRequests covers input validation:
// duplicate TaskIDs and non-positive device counts must be rejected as
// caller errors, not silently accepted or misinterpreted.
func TestPlanGangDeviceIDs_RejectsInvalidRequests(t *testing.T) {
	domain := TopologyDomain{
		ID:            "s1-n2/domain-a",
		DeviceIDs:     []string{"d0", "d1", "d2", "d3"},
		FreeDeviceIDs: []string{"d0", "d1", "d2", "d3"},
	}

	cases := []struct {
		name     string
		requests []GangDeviceRequest
	}{
		{
			name: "duplicate TaskID",
			requests: []GangDeviceRequest{
				{TaskID: "task-a", DevicesPerTask: 1},
				{TaskID: "task-a", DevicesPerTask: 1},
			},
		},
		{
			name: "empty TaskID",
			requests: []GangDeviceRequest{
				{TaskID: "", DevicesPerTask: 1},
			},
		},
		{
			name: "zero DevicesPerTask",
			requests: []GangDeviceRequest{
				{TaskID: "task-a", DevicesPerTask: 0},
			},
		},
		{
			name: "negative DevicesPerTask",
			requests: []GangDeviceRequest{
				{TaskID: "task-a", DevicesPerTask: -1},
			},
		},
		{
			name:     "no requests",
			requests: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := PlanGangDeviceIDs(domain, tc.requests); err == nil {
				t.Fatalf("expected error for case %q, got nil", tc.name)
			}
		})
	}
}