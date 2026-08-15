# Study case: external domain data + scoped call, no vendor changes

For [#5751](https://github.com/volcano-sh/volcano/issues/5751) Case 1, applied to `ascend/mindcluster/ascend310p/vnpu`, which has no per-device domain field.

## What this shows

Two tests, both calling the real, unmodified `NPUDevices.SelectChipFromNode`:

1. **Domain-blind**: 10 chips, no domain restriction, request for 8 whole cards. Succeeds — returns chips `3,8,1,4,5,6,7,9`: 3 from one domain, 5 from the other. Real cross-domain placement from real code.

2. **Domain-scoped**: same 10 chips, same request, but the call is scoped to one externally-supplied 5-chip domain at a time (a `domainProvider` stub simulating what a real `TopologyProvider` would deliver). Both domains correctly refuse: `"free whole chip <5> not enough for req <8>"`.

No changes to `vnpu.NPUDevices`, `vnpu.VChip`, or `SelectChipFromNode`. Domain data and the scoping wrapper are both external to the vendor package.

## What this proves

That supplying domain membership externally and scoping the vendor call to one domain is enough to get correct Case 1 behavior from a vendor plugin that has no domain concept of its own. This is a minimal existence proof, not a production design — no gang planning, no reservation, no rollback.

## What this does NOT prove

- Not a fix. Doesn't touch real Volcano scheduling paths.
- Not evidence about "whole card" requests only — segment (fractional) requests weren't tested here.
- The `domainProvider` stub is illustrative. A real provider would need a
  data source (annotation, CRD, device plugin) — not addressed here.

## Run it

```bash
go test ./study/xpu-domain-mindcluster/... -v

=== RUN   TestMindclusterVnpu_DomainBlindSpansDomains
    domain_study_test.go:109: domain-blind SelectChipFromNode returned chips: 3,8,1,4,5,6,7,9
--- PASS: TestMindclusterVnpu_DomainBlindSpansDomains (0.00s)
=== RUN   TestMindclusterVnpu_DomainScopedCorrectlyRefuses
    domain_study_test.go:132: domain "domain-b" correctly rejected 8-chip request with only 5 chips: selectChipFromNodeWhole free whole chip <5> not enough for req <8>
    domain_study_test.go:132: domain "domain-a" correctly rejected 8-chip request with only 5 chips: selectChipFromNodeWhole free whole chip <5> not enough for req <8>
--- PASS: TestMindclusterVnpu_DomainScopedCorrectlyRefuses (0.00s)
PASS
ok  	volcano.sh/volcano/study/xpu-domain-mindcluster	0.024s
```