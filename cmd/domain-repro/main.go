// Command domain-repro demonstrates the "Case 1" gap described in
// volcano-sh/volcano#5751: a node can report enough free devices in
// aggregate to satisfy a same-domain gang request, while those devices are
// actually fragmented across independent physical domains (HCCS/NVLink/etc),
// so an allocation that only checks the aggregate count silently produces
// a cross-domain placement.
//
// This program deliberately does NOT modify or hook into the scheduler's
// real allocate/preempt path (pkg/scheduler/actions/...). That path's
// device-domain data model is still an open design question as of
// volcano-sh/volcano#5751 (see the DeviceInfo-reuse-vs-new-struct and
// gradient-phase-vs-predicate-phase threads on that issue). Landing code
// against an unsettled interface risks being thrown away. Instead this is a
// standalone, read-only demonstration: it decodes a real node device
// annotation with the REAL devices.UnMarshalNodeDevices function and shows,
// step by step, where "aggregate count is sufficient" and "same-domain
// placement is possible" diverge.
//
// Usage:
//
//	go run ./cmd/domain-repro -devices /tmp/node0-devices.json -request 8
//
// The input file is the raw JSON array produced by
// scripts/create-kwok-nodes-device-domains.sh (i.e. exactly what a real
// vendor plugin would find under its node-register annotation, per
// pkg/scheduler/api/devices/ascend/hami/device_info.go:getNodeDevices).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	// Real, unmodified package from the volcano checkout. Run this command
	// from the root of the volcano module (or with an appropriate replace/
	// GOPATH setup) so this import resolves to your local pkg/scheduler/api/devices.
	"volcano.sh/volcano/pkg/scheduler/api/devices"
)

func main() {
	devicesPath := flag.String("devices", "", "path to a JSON file containing a devices.DeviceInfo array (see create-kwok-nodes-device-domains.sh)")
	requestSize := flag.Int("request", 8, "number of same-domain devices requested by the gang")
	flag.Parse()

	if *devicesPath == "" {
		fmt.Fprintln(os.Stderr, "error: -devices is required")
		flag.Usage()
		os.Exit(2)
	}

	raw, err := os.ReadFile(*devicesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading %s: %v\n", *devicesPath, err)
		os.Exit(1)
	}

	// This is NOT a custom parser. It is the exact function the real
	// Ascend/HAMi device plugin calls in getNodeDevices() to decode a
	// node's register annotation.
	deviceList, err := devices.UnMarshalNodeDevices(string(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: devices.UnMarshalNodeDevices: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Decoded %d devices from node annotation using devices.UnMarshalNodeDevices.\n\n", len(deviceList))

	// --- Step 1: what today's aggregate-count view sees ---
	//
	// This mirrors what a plain resource-quantity check (or a domain-blind
	// device selection loop) does today: it only cares that Count-Used
	// devices exist somewhere on the node, never which physical domain
	// they're in.
	healthy := 0
	for _, d := range deviceList {
		if d.Health {
			healthy++
		}
	}
	fmt.Printf("[Aggregate view]  node has %d healthy devices total.\n", healthy)
	if healthy >= *requestSize {
		fmt.Printf("[Aggregate view]  %d >= requested %d -> node looks SCHEDULABLE.\n\n", healthy, *requestSize)
	} else {
		fmt.Printf("[Aggregate view]  %d < requested %d -> node looks UNSCHEDULABLE. Nothing more to demonstrate.\n", healthy, *requestSize)
		return
	}

	// --- Step 2: what a domain-aware view sees ---
	//
	// Group by CustomInfo["NetworkID"], the exact field already read by
	// hasNetworkID/selectDevicesWithTopology in the existing Ascend/HAMi
	// plugin (pkg/scheduler/api/devices/ascend/hami/device_info.go). This
	// is the closest thing to a "canonical domain identity" that exists in
	// the codebase today, and it is currently only consumed inside that one
	// vendor plugin's own device-selection loop -- never by the generic
	// scheduler cache (pkg/scheduler/cache/event_handlers.go has no
	// device-domain awareness at all).
	byDomain := map[int][]*devices.DeviceInfo{}
	for _, d := range deviceList {
		if !d.Health {
			continue
		}
		domain, ok := domainOf(d)
		if !ok {
			fmt.Printf("warning: device %s has no CustomInfo[\"NetworkID\"], excluding from domain view\n", d.ID)
			continue
		}
		byDomain[domain] = append(byDomain[domain], d)
	}

	domains := make([]int, 0, len(byDomain))
	for k := range byDomain {
		domains = append(domains, k)
	}
	sort.Ints(domains)

	fmt.Println("[Domain view]     per-domain healthy device counts:")
	best := 0
	for _, d := range domains {
		count := len(byDomain[d])
		fmt.Printf("[Domain view]       domain %d: %d devices\n", d, count)
		if count > best {
			best = count
		}
	}
	fmt.Println()

	// --- Step 3: the divergence ---
	if best >= *requestSize {
		fmt.Printf("[Domain view]     largest single domain has %d devices >= requested %d.\n", best, *requestSize)
		fmt.Println("[Domain view]     A same-domain gang request CAN be satisfied -> aggregate and domain views agree here.")
		return
	}

	fmt.Printf("[Domain view]     largest single domain has only %d devices < requested %d.\n", best, *requestSize)
	fmt.Println("[Domain view]     No single domain can satisfy this gang request.")
	fmt.Println()
	fmt.Println("==> GAP DEMONSTRATED: the aggregate view says SCHEDULABLE, the domain view says NOT SCHEDULABLE.")
	fmt.Println("    A domain-blind allocator would place devices from multiple domains for one gang,")
	fmt.Println("    exactly the failure mode described as \"Case 1\" in volcano-sh/volcano#5751.")
}

// domainOf extracts the NetworkID CustomInfo field the same way
// hasNetworkID/selectDevicesWithTopology do in the existing Ascend/HAMi
// plugin: as a JSON-decoded float64.
func domainOf(d *devices.DeviceInfo) (int, bool) {
	if d.CustomInfo == nil {
		return 0, false
	}
	raw, ok := d.CustomInfo["NetworkID"]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}