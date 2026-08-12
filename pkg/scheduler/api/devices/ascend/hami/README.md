# Case 1 (#5751) investigation: selectDevices domain-spanning behavior

## What was tested

`TestSelectDevices_Case1_FragmentedAcrossDomains` in `pkg/scheduler/api/devices/ascend/hami/device_info_test.go`.

Calls the real, unexported `AscendDevices.selectDevices` — the method used by `Allocate`, `FilterNode`, and `ScoreNode` — with:

- 10 fabricated `AscendDevice` entries, 5 tagged `CustomInfo["NetworkID"]: 0`,5 tagged `CustomInfo["NetworkID"]: 1`
- a pod requesting 8 devices of type `huawei.com/Ascend910`

No mocks of `selectDevices` itself. No threshold comparison. The test reads which actual devices came back.

## Run it

```
go test ./pkg/scheduler/api/devices/ascend/hami/... -run TestSelectDevices_Case1 -v

=== RUN   TestSelectDevices_Case1_FragmentedAcrossDomains
    device_info_test.go:1106: selectDevices returned devices from 2 distinct NetworkID domain(s): map[0:5 1:3]
--- PASS: TestSelectDevices_Case1_FragmentedAcrossDomains (0.00s)
```

`selectDevices` returned 5 devices from domain 0 and 3 from domain 1.

## What this shows

`selectDevicesWithTopology` (called from `selectDevices` when all candidate devices carry `NetworkID`) sorts domains by device count descending and fills from the largest domain first. When the largest domain can't fully satisfy the request, it continues into the next domain rather than failing.

This matches the scenario described as Case 1 in #5751: total node capacity is sufficient (10 ≥ 8), no single domain is sufficient (5 < 8), and the
current code returns a cross-domain placement instead of rejecting the request or restricting to one domain.

## What this does NOT show

- Not evidence about any device plugin other than Ascend/HAMi.
- Not evidence this is unintentional. `selectDevicesWithTopology`'s existence shows domain-awareness was deliberately added; whether "prefer fewer domains, allow spanning if needed" is the intended behavior or a gap relative to #5751's goal is unconfirmed.

## Open questions (for #5751 maintainers)

1. Is the current spanning behavior an accepted best-effort trade-off for HAMi, or is strict single-domain placement the actual target?
2. Will the new generic topology model in #5751 supersede this vendor-specific logic, or does the new plugin need to coordinate with existing vendor behavior like this?
