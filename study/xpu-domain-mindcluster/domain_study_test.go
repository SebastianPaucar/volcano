// --- Study case for volcano-sh/volcano#5751 Case 1 ---
//
// This file demonstrates a generic domain-aware layer wrapping a vendor
// plugin that has NO per-device domain concept of its own
// (ascend/mindcluster/ascend310p/vnpu). Unlike the earlier HAMi finding
// (domain data exists but isn't enforced), mindcluster/vnpu's VChip struct
// has no domain/group field at all -- SelectChipFromNode has nothing to
// enforce even in principle.
//
// The point of this file: prove that an EXTERNALLY supplied domain split
// (the kind of data a generic TopologyProvider would deliver) is enough to
// make the REAL, UNMODIFIED SelectChipFromNode correctly refuse a
// fragmented request, by scoping which chips it's allowed to see -- without
// modifying vnpu.NPUDevices, vnpu.VChip, or SelectChipFromNode itself.
//
// This does not propose a production architecture. It's a minimal proof
// that "supply domain data externally, scope the vendor call to one
// domain's devices" is a workable pattern for vendors with no domain field.

package vnpu310p_domain_study

import (
	"fmt"
	"testing"

	"volcano.sh/volcano/pkg/scheduler/api/devices/ascend/mindcluster/ascend310p/vnpu"
)

// --- Minimal provider stub ---
//
// Represents the kind of data a real TopologyProvider would supply:
// which chip IDs belong to which local domain, for a given node. This is
// intentionally tiny -- just enough to drive the study case, not a proposed
// production interface.

type domainProvider struct {
	// domains maps domain name -> chip IDs in that domain.
	domains map[string][]int
}

func (p *domainProvider) Domains() map[string][]int {
	return p.domains
}

// --- Fixture construction ---
//
// Builds a *vnpu.NPUDevices with 10 whole-card chips, matching the real
// struct requirements traced against SelectChipFromNode /
// selectChipFromNodeWhole / IsResourceWholeCard / getVChipCoreNum /
// getWholeCardIDFromAscendReal:
//   - ServerType "Ascend310P-4-dual" so getVChipCoreNum() parses chip core
//     count 4 from the "-"-split ServerType string.
//   - AiCorePerChip 4, matching ServerType's core count (selectChipFromNodeWhole
//     divides by this directly; IsResourceWholeCard derives its own value from
//     ServerType via getVChipCoreNum -- both must agree or IsResourceWholeCard's
//     modulo check silently disagrees with selectChipFromNodeWhole's division).
//   - Chip names "Ascend310P-<id>", matching getWholeCardIDFromAscendReal's
//     strings.Split(name, "-")[1] parsing.
//   - SegmentFlag false on every chip, so isChipVGroupValid short-circuits
//     true (whole-card chips skip vGroup/DVPP checks per the real code).
//   - FreeRes/TotalRes.Aicore 4 per chip, matching vResChip.Aicore computed
//     inside selectChipFromNodeWhole (vRes.Aicore / reqCardNum).

func buildFixture(chipIDs []int) *vnpu.NPUDevices {
	device := vnpu.NewNPUDevices("study-node", nil)
	device.NodeInf.Name = "study-node"
	device.NPUDevice.ServerType = "Ascend310P-4-dual"
	device.NPUDevice.AiCorePerChip = 4
	device.NPUDevice.ChipKind = "Ascend310P"

	device.NPUDevice.Chips = make(map[int]*vnpu.VChip, len(chipIDs))
	for _, id := range chipIDs {
		device.NPUDevice.Chips[id] = &vnpu.VChip{
			Name:        fmt.Sprintf("Ascend310P-%d", id),
			Kind:        "Ascend310P",
			SegmentFlag: false,
			TotalRes:    vnpu.VResource{Aicore: 4, Aicpu: 1, DVPP: "null"},
			FreeRes:     vnpu.VResource{Aicore: 4, Aicpu: 1, DVPP: "null"},
			UsedRes:     vnpu.VResource{Aicore: 0, Aicpu: 0, DVPP: "null"},
		}
	}
	return device
}

// scopedToDomain returns a shallow copy of device whose Chips map is
// restricted to only the given chip IDs. It calls the REAL, unmodified
// SelectChipFromNode on this scoped copy -- no vendor code is modified.
func scopedToDomain(device *vnpu.NPUDevices, chipIDs []int) *vnpu.NPUDevices {
	scoped := *device // shallow copy of NPUDevices (NodeInf, NPUDevice, FrameAttr fields)
	scoped.NPUDevice.Chips = make(map[int]*vnpu.VChip, len(chipIDs))
	for _, id := range chipIDs {
		if chip, ok := device.NPUDevice.Chips[id]; ok {
			scoped.NPUDevice.Chips[id] = chip
		}
	}
	return &scoped
}

func TestMindclusterVnpu_DomainBlindSpansDomains(t *testing.T) {
	device := buildFixture([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}) // 10 chips total

	// Whole-card request for 8 chips: Aicore = 8 * AiCorePerChip(4) = 32.
	req := vnpu.VResource{Aicore: 32, Aicpu: 8, DVPP: "null"}

	result, err := device.SelectChipFromNode(req)
	if err != nil {
		t.Fatalf("expected domain-blind SelectChipFromNode to succeed (spanning domains), got error: %v", err)
	}
	t.Logf("domain-blind SelectChipFromNode returned chips: %s", result)
	// No assertion on WHICH chips -- the point is it succeeds at all,
	// despite having no way to know or care whether the 8 chips it picked
	// span the two 5-chip domains the caller (a real deployment) would
	// actually have. That's the gap: success here is not evidence of a
	// correct, single-domain placement.
}

func TestMindclusterVnpu_DomainScopedCorrectlyRefuses(t *testing.T) {
	device := buildFixture([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	provider := &domainProvider{domains: map[string][]int{
		"domain-a": {0, 1, 2, 3, 4},
		"domain-b": {5, 6, 7, 8, 9},
	}}

	req := vnpu.VResource{Aicore: 32, Aicpu: 8, DVPP: "null"} // 8 chips requested, 5 max per domain

	for domainName, chipIDs := range provider.Domains() {
		scoped := scopedToDomain(device, chipIDs)
		_, err := scoped.SelectChipFromNode(req)
		if err == nil {
			t.Fatalf("expected domain %q (only %d chips) to be rejected for an 8-chip request, but SelectChipFromNode succeeded", domainName, len(chipIDs))
		}
		t.Logf("domain %q correctly rejected 8-chip request with only %d chips: %v", domainName, len(chipIDs), err)
	}
}