# Device-domain aggregate-vs-domain check divergence (illustration for #5751 Case 1)

## What this is

A standalone program that decodes a node's device-register annotation with the real `devices.UnMarshalNodeDevices` / `devices.DeviceInfo` (`pkg/scheduler/api/devices/device_info.go`), then compares two ways of answering "can this node serve an N-device same-domain request":

- **Aggregate check**: `sum(healthy devices) >= N`
- **Domain check**: `max(healthy devices in any single domain) >= N`

Domain membership is read from `CustomInfo["NetworkID"]` — the same field already read by `hasNetworkID` / `selectDevicesWithTopology` in the existing Ascend/HAMi plugin (`pkg/scheduler/api/devices/ascend/hami/device_info.go`).

With a fragmented split (5 devices in domain 0, 5 in domain 1, N=8), the two checks disagree: aggregate says yes, domain says no.

## What this shows

That "enough devices in total" and "enough devices in one domain" are different questions, and a check based only on the first would be wrong. This is a restatement, in runnable code, of the example already given in #5751's Case 1 description (16-NPU node, two 8-NPU domains).

## Run it

```bash
go run ./cmd/domain-repro -devices cmd/domain-repro/testdata-sample-fragmented.json -request 8
```

Fixture: 10 devices total, 5 in domain 0, 5 in domain 1.

## Why N=8 with a 5/5 split, and not other numbers

The request must be: (a) satisfiable by total capacity, so the aggregate
check says yes, and (b) larger than every individual domain, so the domain
check says no. A request larger than total capacity (e.g. N=9 on an 8/8
split) fails both checks for the same reason — that isn't fragmentation,
it's insufficient capacity, and proves nothing about domain-awareness.

## Files

- `cmd/domain-repro/main.go` — the comparison program.
- `cmd/domain-repro/testdata-sample-fragmented.json` — the 5/5-split fixture.
- `benchmark/scripts/create-kwok-nodes-device-domains.sh` — optional, creates a live KWOK node carrying the same annotation shape, if you want to pull the JSON from a real node object instead of a static file.

## Status

Illustration for design discussion on #5751, not a PR, not a claimed fix, not a confirmed bug report against current Volcano behavior.