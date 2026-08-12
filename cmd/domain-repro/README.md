# domain-repro: aggregate-vs-domain check divergence (illustration for #5751 Case 1)

## What this is

A standalone program that decodes a node device-register annotation with the real `devices.UnMarshalNodeDevices` / `devices.DeviceInfo` (`pkg/scheduler/api/devices/device_info.go`), then compares:

- **Aggregate check**: `sum(healthy devices) >= N`
- **Domain check**: `max(healthy devices in any single domain) >= N`

Domain membership is read from `CustomInfo["NetworkID"]` — the field already used by `hasNetworkID` / `selectDevicesWithTopology` in the Ascend/HAMi plugin (`pkg/scheduler/api/devices/ascend/hami/device_info.go`).

With a 5/5 domain split and N=8, the two checks disagree: aggregate says yes (10 ≥ 8), domain says no (max(5,5)=5 < 8).

## Run it

```bash
$ go build ./cmd/domain-repro/

go: downloading k8s.io/api v0.36.1
go: downloading k8s.io/apimachinery v0.36.1
go: downloading k8s.io/client-go v0.36.1
go: downloading k8s.io/klog/v2 v2.140.0
go: downloading k8s.io/utils v0.0.0-20260210185600-b8788abfbbc2
go: downloading sigs.k8s.io/randfill v1.0.0
go: downloading github.com/go-logr/logr v1.4.3
go: downloading k8s.io/kube-openapi v0.0.0-20260317180543-43fb72c5454a
go: downloading sigs.k8s.io/structured-merge-diff/v6 v6.3.2
go: downloading gopkg.in/inf.v0 v0.9.1
go: downloading sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730
go: downloading github.com/json-iterator/go v1.1.12
go: downloading go.yaml.in/yaml/v2 v2.4.4
go: downloading github.com/fxamacker/cbor/v2 v2.9.0
go: downloading golang.org/x/net v0.56.0
go: downloading github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
go: downloading github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee
go: downloading github.com/x448/float16 v0.8.4
go: downloading golang.org/x/text v0.40.0
go: downloading github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822
go: downloading github.com/google/gnostic-models v0.7.0
go: downloading google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af
go: downloading golang.org/x/time v0.15.0
go: downloading golang.org/x/term v0.45.0
go: downloading golang.org/x/oauth2 v0.36.0
go: downloading go.yaml.in/yaml/v3 v3.0.4
go: downloading sigs.k8s.io/yaml v1.6.0
go: downloading github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
go: downloading golang.org/x/sys v0.47.0
go: downloading github.com/google/uuid v1.6.0
go: downloading github.com/go-openapi/swag v0.25.5
go: downloading gopkg.in/evanphx/json-patch.v4 v4.13.0
go: downloading github.com/go-openapi/jsonreference v0.21.4
go: downloading github.com/emicklei/go-restful/v3 v3.13.0
go: downloading github.com/go-openapi/jsonpointer v0.22.5
go: downloading github.com/go-openapi/swag/cmdutils v0.25.5
go: downloading github.com/go-openapi/swag/conv v0.25.5
go: downloading github.com/go-openapi/swag/fileutils v0.25.5
go: downloading github.com/go-openapi/swag/jsonname v0.25.5
go: downloading github.com/go-openapi/swag/jsonutils v0.25.5
go: downloading github.com/go-openapi/swag/loading v0.25.5
go: downloading github.com/go-openapi/swag/mangling v0.25.5
go: downloading github.com/go-openapi/swag/netutils v0.25.5
go: downloading github.com/go-openapi/swag/stringutils v0.25.5
go: downloading github.com/go-openapi/swag/typeutils v0.25.5
go: downloading github.com/go-openapi/swag/yamlutils v0.25.5

$ go run ./cmd/domain-repro -devices cmd/domain-repro/testdata-sample-fragmented.json -request 8

Decoded 10 devices from node annotation using devices.UnMarshalNodeDevices.

[Aggregate view]  node has 10 healthy devices total.
[Aggregate view]  10 >= requested 8 -> node looks SCHEDULABLE.

[Domain view]     per-domain healthy device counts:
[Domain view]       domain 0: 5 devices
[Domain view]       domain 1: 5 devices

[Domain view]     largest single domain has only 5 devices < requested 8.
[Domain view]     No single domain can satisfy this gang request.

==> GAP DEMONSTRATED: the aggregate view says SCHEDULABLE, the domain view says NOT SCHEDULABLE.
    A domain-blind allocator would place devices from multiple domains for one gang,
    exactly the failure mode described as "Case 1" in volcano-sh/volcano#5751.

```

## Why 5/5 split, N=8

The request must be satisfiable by total capacity but larger than every individual domain. A request larger than total capacity (e.g. N=9 on an
8/8 split) fails both checks for the same reason — insufficient capacity, not fragmentation — and proves nothing.

## What this does NOT show

- Whether Volcano's real allocator has this behavior. This program does not call any scheduler or device-plugin code — it only compares two
  numbers computed from a static JSON fixture.
- A confirmed bug. It restates the aggregate-vs-domain distinction from #5751's own Case 1 description in runnable form; nothing more.

See `pkg/scheduler/api/devices/ascend/hami/README.md`, which calls the real `selectDevices` allocation path (not a threshold comparison) and shows it returning devices split 5/3 across two domains for an 8-device request. Read that instead if you want evidence against shipped code — this program is a simpler illustration of the same underlying question.

## Status

Illustration for #5751 design discussion. Not a PR. Not a claimed fix.