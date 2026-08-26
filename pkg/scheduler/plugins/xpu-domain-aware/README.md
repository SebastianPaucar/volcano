# `xpu-domain-aware` — PoC

Standalone proof-of-concept for [volcano-sh/volcano#5751](https://github.com/volcano-sh/volcano/issues/5751) ("Generic xPU Topology-Aware Scheduling"), built to validate Jesse Stutler's stated direction against real Volcano scheduler code:

- *"Add domain related info before HyperNode/Node selection"*
- *"Build a generic, device-agnostic solution... the final XPU topology-aware scheduling mechanism must be universal"*

## What it does

Registers on `AddHyperNodeGradientForSubJobFn` — the same extension point `network-topology-aware.go` uses — and filters candidate HyperNodes down to ones containing a device domain with enough free devices for the SubJob's request. This runs inside `allocateForSubJob`, **before** `PredicateNodes` or any device `Allocate()` call executes.

Topology comes from a minimal, vendor-neutral interface:

```go
type TopologyProvider interface {
    DomainsForNode(nodeName string) ([]TopologyDomain, bool)
    Refresh() error
}
```

A `MockTopologyProvider` backs it for this PoC, consistent with the project's own "mock topology data and KWOK" allowance. The plugin has no HAMi/mindcluster/vgpu-specific code anywhere — swapping the mock for a real annotation/CRD/DRA-based provider wouldn't require touching the plugin.

## Files

| File | Purpose |
|---|---|
| `topology.go` | `TopologyDomain`, `TopologyProvider` interface, `MockTopologyProvider` |
| `xpu_domain_aware.go` | The gradient-filtering plugin (`PluginName = "xpu-domain-aware"`) |
| `xpu_domain_aware_test.go` | Two tests against real `allocate`/`predicates`/`gang` plugins |

## Tests

**`TestXPUDomainAware_FiltersHyperNodeWithSplitDomains`**
Two HyperNodes both report 8 idle devices in aggregate. One has them split 4/4 across two domains (no single domain can satisfy an 8-device request); the other has one domain covering all 8. Without domain awareness these look identical to the scheduler. With the plugin, only the genuinely satisfying HyperNode survives the gradient and receives the bind — confirmed via full trace: the split-domain HyperNode is rejected at the gradient stage and never reaches `PredicateNodes`.

**`TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation`**
No HyperNode anywhere has a domain that satisfies the request. The SubJob finds no gradient solution and is not bound — instead of the current behavior (bind blindly, discover the domain mismatch at device-allocation time, after the Node/HyperNode are already committed).

## Run it

```bash
go build ./pkg/scheduler/plugins/xpu-domain-aware/...
go vet ./pkg/scheduler/plugins/xpu-domain-aware/...
go test ./pkg/scheduler/plugins/xpu-domain-aware/... -v -run TestXPUDomainAware

=== RUN   TestXPUDomainAware_FiltersHyperNodeWithSplitDomains
--- PASS: TestXPUDomainAware_FiltersHyperNodeWithSplitDomains (0.38s)
=== RUN   TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation
--- PASS: TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation (0.36s)
PASS
ok  	volcano.sh/volcano/pkg/scheduler/plugins/xpu-domain-aware	0.789s
=== RUN   TestXPUDomainAware_FiltersHyperNodeWithSplitDomains
I0825 19:05:49.186446  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.186801  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.186835  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.186919  600478 reflector.go:425] "Starting reflector" type="*v1.VolumeAttachment" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186933  600478 reflector.go:425] "Starting reflector" type="*v1.ResourceQuota" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186932  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.186975  600478 reflector.go:472] "Listing and watching" type="*v1.ResourceQuota" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186836  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.186975  600478 reflector.go:425] "Starting reflector" type="*v1.ResourceSlice" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187040  600478 reflector.go:472] "Listing and watching" type="*v1.ResourceSlice" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187002  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187059  600478 reflector.go:425] "Starting reflector" type="*v1.DeviceClass" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187088  600478 reflector.go:425] "Starting reflector" type="*v1.StorageClass" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186455  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187143  600478 reflector.go:472] "Listing and watching" type="*v1.StorageClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186941  600478 reflector.go:472] "Listing and watching" type="*v1.VolumeAttachment" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187164  600478 reflector.go:425] "Starting reflector" type="*v1.CSINode" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186600  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187206  600478 reflector.go:507] "Caches populated" type="*v1.ResourceSlice" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187218  600478 reflector.go:472] "Listing and watching" type="*v1.CSINode" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187241  600478 reflector.go:425] "Starting reflector" type="*v1.PersistentVolumeClaim" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186884  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187269  600478 reflector.go:507] "Caches populated" type="*v1.ResourceQuota" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187288  600478 reflector.go:425] "Starting reflector" type="*v1.StatefulSet" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187290  600478 reflector.go:472] "Listing and watching" type="*v1.PersistentVolumeClaim" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186760  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187320  600478 reflector.go:472] "Listing and watching" type="*v1.StatefulSet" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187055  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187144  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187200  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187415  600478 reflector.go:425] "Starting reflector" type="*v1beta1.Queue" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187426  600478 reflector.go:472] "Listing and watching" type="*v1beta1.Queue" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187428  600478 reflector.go:507] "Caches populated" type="*v1.CSINode" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187439  600478 reflector.go:425] "Starting reflector" type="*v1.ReplicationController" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187124  600478 reflector.go:472] "Listing and watching" type="*v1.DeviceClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187460  600478 reflector.go:425] "Starting reflector" type="*v1beta1.PodGroup" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187499  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.186729  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187539  600478 reflector.go:507] "Caches populated" type="*v1.PersistentVolumeClaim" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187530  600478 reflector.go:472] "Listing and watching" type="*v1beta1.PodGroup" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187581  600478 reflector.go:425] "Starting reflector" type="*v1.Node" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187590  600478 reflector.go:425] "Starting reflector" type="*v1alpha1.HyperNode" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187602  600478 reflector.go:507] "Caches populated" type="*v1.StatefulSet" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187609  600478 reflector.go:472] "Listing and watching" type="*v1.Node" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187621  600478 reflector.go:472] "Listing and watching" type="*v1alpha1.HyperNode" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187292  600478 reflector.go:425] "Starting reflector" type="*v1.ResourceClaim" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187649  600478 reflector.go:472] "Listing and watching" type="*v1.ResourceClaim" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187552  600478 reflector.go:507] "Caches populated" type="*v1beta1.Queue" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186487  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.186490  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.186503  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187738  600478 reflector.go:507] "Caches populated" type="*v1.DeviceClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187782  600478 reflector.go:425] "Starting reflector" type="*v1.Pod" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187770  600478 reflector.go:425] "Starting reflector" type="*v1.Namespace" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187810  600478 reflector.go:472] "Listing and watching" type="*v1.Pod" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187822  600478 reflector.go:507] "Caches populated" type="*v1beta1.PodGroup" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187827  600478 reflector.go:425] "Starting reflector" type="*v1.PersistentVolume" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187829  600478 reflector.go:507] "Caches populated" type="*v1.VolumeAttachment" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187872  600478 reflector.go:472] "Listing and watching" type="*v1.PersistentVolume" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186551  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187937  600478 reflector.go:507] "Caches populated" type="*v1.Pod" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187828  600478 reflector.go:472] "Listing and watching" type="*v1.Namespace" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187960  600478 reflector.go:425] "Starting reflector" type="*v1.CSIStorageCapacity" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186574  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.186603  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.187992  600478 reflector.go:472] "Listing and watching" type="*v1.CSIStorageCapacity" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188013  600478 reflector.go:507] "Caches populated" type="*v1.PersistentVolume" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188028  600478 reflector.go:425] "Starting reflector" type="*v1.PodDisruptionBudget" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186662  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.188057  600478 reflector.go:472] "Listing and watching" type="*v1.PodDisruptionBudget" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188041  600478 reflector.go:425] "Starting reflector" type="*v1.PriorityClass" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188074  600478 reflector.go:425] "Starting reflector" type="*v1.CSIDriver" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.186754  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.188096  600478 reflector.go:472] "Listing and watching" type="*v1.PriorityClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188133  600478 reflector.go:507] "Caches populated" type="*v1.Node" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188160  600478 reflector.go:507] "Caches populated" type="*v1.CSIStorageCapacity" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188033  600478 reflector.go:507] "Caches populated" type="*v1.Namespace" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188218  600478 reflector.go:507] "Caches populated" type="*v1.PodDisruptionBudget" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188263  600478 reflector.go:507] "Caches populated" type="*v1.PriorityClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188177  600478 reflector.go:425] "Starting reflector" type="*v1.ReplicaSet" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188318  600478 reflector.go:472] "Listing and watching" type="*v1.ReplicaSet" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187350  600478 reflector.go:425] "Starting reflector" type="*v1.Service" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188394  600478 reflector.go:472] "Listing and watching" type="*v1.Service" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187377  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.188431  600478 reflector.go:507] "Caches populated" type="*v1.ReplicaSet" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188514  600478 reflector.go:425] "Starting reflector" type="*v1alpha1.Numatopology" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188524  600478 reflector.go:507] "Caches populated" type="*v1.Service" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187471  600478 reflector.go:472] "Listing and watching" type="*v1.ReplicationController" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188544  600478 reflector.go:472] "Listing and watching" type="*v1alpha1.Numatopology" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188608  600478 reflector.go:507] "Caches populated" type="*v1.ReplicationController" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187618  600478 reflector.go:507] "Caches populated" type="*v1.StorageClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187763  600478 reflector.go:507] "Caches populated" type="*v1.ResourceClaim" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.187902  600478 reflector.go:507] "Caches populated" type="*v1alpha1.HyperNode" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188689  600478 reflector.go:507] "Caches populated" type="*v1alpha1.Numatopology" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188088  600478 reflector.go:472] "Listing and watching" type="*v1.CSIDriver" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188772  600478 reflector.go:507] "Caches populated" type="*v1.CSIDriver" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.188992  600478 cache.go:877] Start metrics collection, metricsConf is map[]
I0825 19:05:49.189025  600478 cache.go:882] The interval for querying metrics data is 30s
I0825 19:05:49.189061  600478 cache.go:889] skip waiting for handlers sync
I0825 19:05:49.189145  600478 cache.go:1779] The metrics type is not set in the volcano scheduler configmap file. As a result, the CPU and memory load information of the node is not collected.
I0825 19:05:49.189462  600478 node_info.go:387] ignored_list is []
I0825 19:05:49.189500  600478 node_info.go:391] ignoredDevicesList is []
I0825 19:05:49.189562  600478 node_info.go:387] ignored_list is []
I0825 19:05:49.189582  600478 node_info.go:391] ignoredDevicesList is []
I0825 19:05:49.189797  600478 event_handlers.go:471] Added pod <c1/p1> into cache.
I0825 19:05:49.189837  600478 event_handlers.go:916] Add PodGroup(pg1) into cache, spec(v1beta1.PodGroupSpec{MinMember:1, MinTaskMember:map[string]int32(nil), Queue:"q1", PriorityClassName:"", MinResources:(*v1.ResourceList)(nil), NetworkTopology:(*v1beta1.NetworkTopologySpec)(0x20656d4ff0e0), SubGroupPolicy:[]v1beta1.SubGroupPolicySpec(nil)})
I0825 19:05:49.189931  600478 event_handlers.go:1009] Add Queue(q1) into cache, spec(v1beta1.QueueSpec{Weight:1, Capability:v1.ResourceList(nil), Reclaimable:(*bool)(nil), ExtendClusters:[]v1beta1.Cluster(nil), Guarantee:v1beta1.Guarantee{Resource:v1.ResourceList(nil)}, Affinity:(*v1beta1.Affinity)(nil), Type:"", Parent:"", Deserved:v1.ResourceList(nil), Priority:0, DequeueStrategy:""})
I0825 19:05:49.190024  600478 node_info.go:191] set numa scheduler info of node s0-n1 to nil
I0825 19:05:49.190040  600478 node_info.go:191] set numa scheduler info of node s1-n2 to nil
I0825 19:05:49.190098  600478 node_info.go:387] ignored_list is []
I0825 19:05:49.190119  600478 node_info.go:391] ignoredDevicesList is []
I0825 19:05:49.190141  600478 node_info.go:248] imageStates is map[]
I0825 19:05:49.190199  600478 node_info.go:387] ignored_list is []
I0825 19:05:49.190215  600478 node_info.go:391] ignoredDevicesList is []
I0825 19:05:49.190234  600478 node_info.go:248] imageStates is map[]
I0825 19:05:49.190383  600478 cache.go:1548] The priority of job <c1/pg1> is </0>
I0825 19:05:49.190475  600478 cache.go:1584] "SnapShot for scheduling" jobNum=1 QueueNum=1 NodeNum=2
I0825 19:05:49.190589  600478 cache.go:1587] "HyperNode snapShot for scheduling" tiers={"0":{"s0":{},"s1":{}}} realNodesSet={"s0":{"s0-n1":{}},"s1":{"s1-n2":{}}} hyperNodesReadyToSchedule=true
I0825 19:05:49.190666  600478 session.go:1046] Start adjusting jobs' network topology spec according to hyperNodeTierNameMap map[]
I0825 19:05:49.190710  600478 session.go:1089] Finish adjusting jobs' network topology spec according to hyperNodeTierNameMap map[]
I0825 19:05:49.190774  600478 session.go:264] "hyperNode in session" name="s0" tier=0 parent="<cluster-top-hypernode>" children={}
I0825 19:05:49.190808  600478 session.go:264] "hyperNode in session" name="s1" tier=0 parent="<cluster-top-hypernode>" children={}
I0825 19:05:49.190838  600478 session.go:264] "hyperNode in session" name="<cluster-top-hypernode>" tier=1 parent="" children={"s0":{},"s1":{}}
I0825 19:05:49.190869  600478 session.go:280] Open Session 83645c74-71d5-4231-a4d2-68cee1cbcbfa with <1> Job and <1> Queues. HyperNodesReadyToSchedule: true
I0825 19:05:49.197436  600478 factory.go:60] Register preBinder predicates successfully
E0825 19:05:49.197492  600478 xpu_domain_aware.go:91] XPU-DOMAIN-AWARE-DEBUG: OnSessionOpen called, devicesPerTask=8
I0825 19:05:49.197528  600478 job_info.go:1091] job pg1/c1 actual: map[:1], ji.TaskMinAvailable: map[]
I0825 19:05:49.197571  600478 allocate.go:123] Enter Allocate ...
I0825 19:05:49.197588  600478 job_info.go:1091] job pg1/c1 actual: map[:1], ji.TaskMinAvailable: map[]
I0825 19:05:49.197633  600478 allocate.go:193] Added Job <c1/pg1> into Queue <q1>
I0825 19:05:49.197652  600478 allocate.go:138] Try to allocate resource to 1 Queues
I0825 19:05:49.197693  600478 allocate.go:311] "Try to allocate resource for job contains hard topology or subjob policy" queue="q1" job="c1/pg1" allocatedHyperNode="" subJobNum=1
I0825 19:05:49.197725  600478 allocate.go:393] "Try to allocate resource for job in hyperNode" job="c1/pg1" hyperNode="<cluster-top-hypernode>"
I0825 19:05:49.197767  600478 allocate.go:474] "Try to allocate resource for subJob" job="c1/pg1" subJob="c1/pg1" allocatedHyperNode="" nominatedHyperNode="" taskNum=1
E0825 19:05:49.197786  600478 xpu_domain_aware.go:121] XPU-DOMAIN-AWARE-DEBUG: REJECT hyperNode=s0 subJob=c1/pg1 devicesPerTask=8
E0825 19:05:49.197800  600478 xpu_domain_aware.go:118] XPU-DOMAIN-AWARE-DEBUG: ACCEPT hyperNode=s1 subJob=c1/pg1
E0825 19:05:49.197815  600478 xpu_domain_aware.go:125] XPU-DOMAIN-AWARE-DEBUG: gradient fn done, survivors=1
I0825 19:05:49.197843  600478 allocate.go:494] "Try to allocate resource for tasks in subJob" job="c1/pg1" subJob="c1/pg1" taskNum=1 hyperNode="s1"
I0825 19:05:49.197890  600478 allocate.go:777] There are <1> nodes for Job <c1/pg1>
I0825 19:05:49.197914  600478 predicates.go:937] The predicate of plugin NodeAffinity will skip execution for pod <c1/p1>, because the status returned by pre-predicate is skip
I0825 19:05:49.197932  600478 predicates.go:937] The predicate of plugin NodePorts will skip execution for pod <c1/p1>, because the status returned by pre-predicate is skip
I0825 19:05:49.197974  600478 plugin.go:167] "getting namespace, assuming empty set of namespace labels" namespace="c1" err="namespace \"c1\" not found"
I0825 19:05:49.197996  600478 predicates.go:937] The predicate of plugin InterPodAffinity will skip execution for pod <c1/p1>, because the status returned by pre-predicate is skip
I0825 19:05:49.198009  600478 predicates.go:937] The predicate of plugin NodeVolumeLimits will skip execution for pod <c1/p1>, because the status returned by pre-predicate is skip
I0825 19:05:49.198020  600478 predicates.go:937] The predicate of plugin VolumeZone will skip execution for pod <c1/p1>, because the status returned by pre-predicate is skip
I0825 19:05:49.198032  600478 predicates.go:937] The predicate of plugin PodTopologySpread will skip execution for pod <c1/p1>, because the status returned by pre-predicate is skip
I0825 19:05:49.198044  600478 predicates.go:937] The predicate of plugin VolumeBinding will skip execution for pod <c1/p1>, because the status returned by pre-predicate is skip
I0825 19:05:49.198090  600478 dynamicresources.go:481] "pod resource claims" pod="c1/p1" resourceclaims=[]
I0825 19:05:49.198152  600478 predicates.go:937] The predicate of plugin DynamicResources will skip execution for pod <c1/p1>, because the status returned by pre-predicate is skip
I0825 19:05:49.198199  600478 predicate_helper.go:84] Considering Task <c1/p1> on node <s1-n2>: <cpu 0.00, memory 0.00, example.com/xpu 8000.00, pods 1.00> vs. <cpu 8000.00, memory 17179869184.00, example.com/xpu 8000.00, pods 10.00>
I0825 19:05:49.198290  600478 allocate.go:928] node s1-n2, idle: cpu 8000.00, memory 17179869184.00, example.com/xpu 8000.00, pods 10.00, future idle: cpu 8000.00, memory 17179869184.00, example.com/xpu 8000.00, pods 10.00
I0825 19:05:49.198338  600478 allocate.go:956] Binding Task <c1/p1> to node <s1-n2>
I0825 19:05:49.198380  600478 statement.go:287] After allocated Task <c1/p1> to Node <s1-n2>: idle <cpu 8000.00, memory 17179869184.00, example.com/xpu 0.00, pods 9.00>, used <cpu 0.00, memory 0.00, example.com/xpu 8000.00, pods 1.00>, releasing <cpu 0.00, memory 0.00>
I0825 19:05:49.198414  600478 predicates.go:215] predicates, allocate s1-n2
I0825 19:05:49.198518  600478 predicates.go:260] predicates, update pod c1/p1 allocate to node [s1-n2]
I0825 19:05:49.198543  600478 statement.go:317] Allocating operations ...
I0825 19:05:49.198595  600478 allocate.go:855] "SubJob ready, return statement" job="c1/pg1" subJob="c1/pg1"
I0825 19:05:49.198614  600478 statement.go:500] Save operations: task p1 allocate from node s1-n2 
I0825 19:05:49.198630  600478 statement.go:376] Discarding operations ...
I0825 19:05:49.198652  600478 statement.go:357] Remove Task <p1> on node <s1-n2>
I0825 19:05:49.198668  600478 predicates.go:263] predicates, deallocate s1-n2
I0825 19:05:49.198691  600478 predicates.go:311] predicates, update pod c1/p1 deallocate from node [s1-n2]
I0825 19:05:49.198756  600478 scheduler_helper.go:175] "Prioritize hyperNode score map for subJob" subJob="c1/pg1" scoreMap={"s1":0}
I0825 19:05:49.198775  600478 statement.go:500] Recover operations: 
I0825 19:05:49.198804  600478 statement.go:287] After allocated Task <c1/p1> to Node <s1-n2>: idle <cpu 8000.00, memory 17179869184.00, example.com/xpu 0.00, pods 9.00>, used <cpu 0.00, memory 0.00, example.com/xpu 8000.00, pods 1.00>, releasing <cpu 0.00, memory 0.00>
I0825 19:05:49.198841  600478 predicates.go:215] predicates, allocate s1-n2
I0825 19:05:49.198892  600478 predicates.go:260] predicates, update pod c1/p1 allocate to node [s1-n2]
I0825 19:05:49.198915  600478 statement.go:317] Allocating operations ...
I0825 19:05:49.198957  600478 allocate.go:531] "Allocate subJob to hyperNode success" subJob="c1/pg1" hyperNode="s1" score=0 newAllocatedHyperNode="s1"
I0825 19:05:49.198978  600478 statement.go:500] Save operations: task p1 allocate from node s1-n2 
I0825 19:05:49.198997  600478 statement.go:376] Discarding operations ...
I0825 19:05:49.199019  600478 statement.go:357] Remove Task <p1> on node <s1-n2>
I0825 19:05:49.199034  600478 predicates.go:263] predicates, deallocate s1-n2
I0825 19:05:49.199055  600478 predicates.go:311] predicates, update pod c1/p1 deallocate from node [s1-n2]
I0825 19:05:49.199070  600478 statement.go:500] Recover operations: 
I0825 19:05:49.199097  600478 statement.go:287] After allocated Task <c1/p1> to Node <s1-n2>: idle <cpu 8000.00, memory 17179869184.00, example.com/xpu 0.00, pods 9.00>, used <cpu 0.00, memory 0.00, example.com/xpu 8000.00, pods 1.00>, releasing <cpu 0.00, memory 0.00>
I0825 19:05:49.199129  600478 predicates.go:215] predicates, allocate s1-n2
I0825 19:05:49.199174  600478 predicates.go:260] predicates, update pod c1/p1 allocate to node [s1-n2]
I0825 19:05:49.199194  600478 statement.go:317] Allocating operations ...
I0825 19:05:49.199231  600478 allocate.go:456] "Allocate job to hyperNode success" job="c1/pg1" hyperNode="<cluster-top-hypernode>"
I0825 19:05:49.199248  600478 statement.go:403] Committing operations ...
I0825 19:05:49.199266  600478 cache.go:1345] add bind task c1/p1
I0825 19:05:49.199332  600478 recorder.go:66] "update allocated hyperNode for job" job="c1/pg1" old="" new="<cluster-top-hypernode>"
I0825 19:05:49.199353  600478 recorder.go:79] "update allocated hyperNode for subJob" subJob="c1/pg1" old="" new="s1"
I0825 19:05:49.199391  600478 allocate.go:301] Can not find jobs for queue q1.
I0825 19:05:49.199406  600478 allocate.go:140] Leaving Allocate ...
I0825 19:05:49.209181  600478 cache.go:1459] batch bind task count 1
I0825 19:05:49.209249  600478 cache.go:992] bind ok, latency 10.601µs
I0825 19:05:49.560595  600478 session.go:578] Close Session 83645c74-71d5-4231-a4d2-68cee1cbcbfa
--- PASS: TestXPUDomainAware_FiltersHyperNodeWithSplitDomains (0.38s)
I0825 19:05:49.560715  600478 watch.go:218] "Stopping fake watcher"
=== RUN   TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation
I0825 19:05:49.560789  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.560816  600478 reflector.go:434] "Stopping reflector" type="*v1.PriorityClass" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560794  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.560872  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.560872  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.560922  600478 reflector.go:434] "Stopping reflector" type="*v1.ReplicaSet" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560925  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.560961  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.560935  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.560970  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561008  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.560783  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561018  600478 reflector.go:434] "Stopping reflector" type="*v1.PodDisruptionBudget" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561036  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561006  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561064  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561077  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561098  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561105  600478 reflector.go:434] "Stopping reflector" type="*v1.ReplicationController" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561123  600478 reflector.go:434] "Stopping reflector" type="*v1.PersistentVolume" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561142  600478 reflector.go:434] "Stopping reflector" type="*v1.VolumeAttachment" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560985  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561196  600478 reflector.go:434] "Stopping reflector" type="*v1alpha1.Numatopology" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560874  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561250  600478 reflector.go:434] "Stopping reflector" type="*v1.PersistentVolumeClaim" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560961  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561321  600478 reflector.go:434] "Stopping reflector" type="*v1.CSINode" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560833  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561386  600478 reflector.go:434] "Stopping reflector" type="*v1beta1.PodGroup" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561126  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561454  600478 reflector.go:434] "Stopping reflector" type="*v1.StatefulSet" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560900  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561508  600478 reflector.go:434] "Stopping reflector" type="*v1.DeviceClass" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561102  600478 reflector.go:434] "Stopping reflector" type="*v1.CSIStorageCapacity" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561063  600478 reflector.go:434] "Stopping reflector" type="*v1.StorageClass" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561038  600478 reflector.go:434] "Stopping reflector" type="*v1.ResourceQuota" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560994  600478 reflector.go:434] "Stopping reflector" type="*v1.Service" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560924  600478 reflector.go:434] "Stopping reflector" type="*v1.ResourceClaim" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560966  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561722  600478 reflector.go:434] "Stopping reflector" type="*v1.CSIDriver" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560983  600478 reflector.go:434] "Stopping reflector" type="*v1beta1.Queue" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.560922  600478 reflector.go:434] "Stopping reflector" type="*v1.Node" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561005  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.561888  600478 reflector.go:434] "Stopping reflector" type="*v1.ResourceSlice" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561054  600478 reflector.go:434] "Stopping reflector" type="*v1alpha1.HyperNode" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561056  600478 reflector.go:434] "Stopping reflector" type="*v1.Pod" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.561105  600478 reflector.go:434] "Stopping reflector" type="*v1.Namespace" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562016  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562013  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562067  600478 reflector.go:425] "Starting reflector" type="*v1.CSIStorageCapacity" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562065  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562094  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562145  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562089  600478 reflector.go:472] "Listing and watching" type="*v1.CSIStorageCapacity" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562199  600478 reflector.go:425] "Starting reflector" type="*v1.ResourceQuota" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562235  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562108  600478 reflector.go:425] "Starting reflector" type="*v1.PersistentVolumeClaim" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562256  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562281  600478 reflector.go:472] "Listing and watching" type="*v1.PersistentVolumeClaim" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562289  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562134  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562322  600478 reflector.go:425] "Starting reflector" type="*v1.PodDisruptionBudget" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562336  600478 reflector.go:507] "Caches populated" type="*v1.CSIStorageCapacity" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562349  600478 reflector.go:425] "Starting reflector" type="*v1.CSINode" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562365  600478 reflector.go:425] "Starting reflector" type="*v1.Namespace" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562157  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562402  600478 reflector.go:472] "Listing and watching" type="*v1.Namespace" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562410  600478 reflector.go:507] "Caches populated" type="*v1.PersistentVolumeClaim" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562437  600478 reflector.go:425] "Starting reflector" type="*v1.ReplicationController" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562445  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562458  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562471  600478 reflector.go:472] "Listing and watching" type="*v1.ReplicationController" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562462  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562014  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562510  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562538  600478 reflector.go:425] "Starting reflector" type="*v1.VolumeAttachment" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562553  600478 reflector.go:425] "Starting reflector" type="*v1.ReplicaSet" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562565  600478 reflector.go:425] "Starting reflector" type="*v1.Pod" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562579  600478 reflector.go:472] "Listing and watching" type="*v1.VolumeAttachment" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562330  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562605  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562610  600478 reflector.go:472] "Listing and watching" type="*v1.Pod" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562240  600478 reflector.go:472] "Listing and watching" type="*v1.ResourceQuota" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562636  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562280  600478 reflector.go:425] "Starting reflector" type="*v1alpha1.HyperNode" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562675  600478 reflector.go:425] "Starting reflector" type="*v1.StatefulSet" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562688  600478 reflector.go:472] "Listing and watching" type="*v1alpha1.HyperNode" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562704  600478 reflector.go:507] "Caches populated" type="*v1.VolumeAttachment" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562583  600478 reflector.go:472] "Listing and watching" type="*v1.ReplicaSet" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562732  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562731  600478 reflector.go:425] "Starting reflector" type="*v1alpha1.Numatopology" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562544  600478 reflector.go:507] "Caches populated" type="*v1.Namespace" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562353  600478 reflector.go:472] "Listing and watching" type="*v1.PodDisruptionBudget" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562792  600478 reflector.go:472] "Listing and watching" type="*v1alpha1.Numatopology" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562307  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562282  600478 reflector.go:425] "Starting reflector" type="*v1.CSIDriver" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562850  600478 reflector.go:507] "Caches populated" type="*v1.ReplicaSet" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562866  600478 reflector.go:425] "Starting reflector" type="*v1.PersistentVolume" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562735  600478 reflector.go:507] "Caches populated" type="*v1.ResourceQuota" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562888  600478 reflector.go:507] "Caches populated" type="*v1.PodDisruptionBudget" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562908  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562912  600478 reflector.go:507] "Caches populated" type="*v1alpha1.Numatopology" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562650  600478 reflector.go:425] "Starting reflector" type="*v1.ResourceSlice" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562943  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562158  600478 reflector.go:425] "Starting reflector" type="*v1.PriorityClass" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562958  600478 reflector.go:425] "Starting reflector" type="*v1.Service" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562711  600478 reflector.go:472] "Listing and watching" type="*v1.StatefulSet" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562985  600478 reflector.go:472] "Listing and watching" type="*v1.Service" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562984  600478 reflector.go:472] "Listing and watching" type="*v1.PriorityClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562896  600478 reflector.go:472] "Listing and watching" type="*v1.PersistentVolume" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563004  600478 reflector.go:425] "Starting reflector" type="*v1.DeviceClass" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562353  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.563042  600478 reflector.go:472] "Listing and watching" type="*v1.DeviceClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562492  600478 reflector.go:425] "Starting reflector" type="*v1beta1.Queue" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563071  600478 reflector.go:425] "Starting reflector" type="*v1beta1.PodGroup" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563091  600478 reflector.go:472] "Listing and watching" type="*v1beta1.PodGroup" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563093  600478 reflector.go:472] "Listing and watching" type="*v1beta1.Queue" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562798  600478 reflector.go:507] "Caches populated" type="*v1.Pod" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563091  600478 reflector.go:507] "Caches populated" type="*v1.StatefulSet" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563126  600478 reflector.go:507] "Caches populated" type="*v1.PriorityClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562722  600478 reflector.go:363] "The client used to build this informer/reflector doesn't support WatchList semantics. The feature will be disabled. This is expected in unit tests but not in production. For details, see the documentation of watchlist.DoesClientNotSupportWatchListSemantics()." logger="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343" feature="WatchListClient"
I0825 19:05:49.562790  600478 reflector.go:507] "Caches populated" type="*v1alpha1.HyperNode" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563183  600478 reflector.go:507] "Caches populated" type="*v1.Service" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563188  600478 reflector.go:507] "Caches populated" type="*v1beta1.PodGroup" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563213  600478 reflector.go:425] "Starting reflector" type="*v1.StorageClass" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563253  600478 reflector.go:472] "Listing and watching" type="*v1.StorageClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562871  600478 reflector.go:472] "Listing and watching" type="*v1.CSIDriver" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562379  600478 reflector.go:472] "Listing and watching" type="*v1.CSINode" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563356  600478 reflector.go:507] "Caches populated" type="*v1.StorageClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563378  600478 reflector.go:507] "Caches populated" type="*v1.CSIDriver" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563410  600478 reflector.go:507] "Caches populated" type="*v1.CSINode" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562968  600478 reflector.go:472] "Listing and watching" type="*v1.ResourceSlice" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563109  600478 reflector.go:507] "Caches populated" type="*v1.PersistentVolume" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562573  600478 reflector.go:425] "Starting reflector" type="*v1.ResourceClaim" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563497  600478 reflector.go:472] "Listing and watching" type="*v1.ResourceClaim" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563512  600478 reflector.go:507] "Caches populated" type="*v1.ResourceSlice" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562596  600478 reflector.go:507] "Caches populated" type="*v1.ReplicationController" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563146  600478 reflector.go:507] "Caches populated" type="*v1.DeviceClass" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.562805  600478 reflector.go:425] "Starting reflector" type="*v1.Node" resyncPeriod="0s" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563599  600478 reflector.go:472] "Listing and watching" type="*v1.Node" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563646  600478 reflector.go:507] "Caches populated" type="*v1.ResourceClaim" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563223  600478 reflector.go:507] "Caches populated" type="*v1beta1.Queue" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563667  600478 reflector.go:507] "Caches populated" type="*v1.Node" reflector="pkg/mod/k8s.io/client-go@v0.36.1/tools/cache/reflector.go:343"
I0825 19:05:49.563905  600478 cache.go:877] Start metrics collection, metricsConf is map[]
I0825 19:05:49.563940  600478 cache.go:882] The interval for querying metrics data is 30s
I0825 19:05:49.563976  600478 cache.go:889] skip waiting for handlers sync
I0825 19:05:49.564065  600478 cache.go:1779] The metrics type is not set in the volcano scheduler configmap file. As a result, the CPU and memory load information of the node is not collected.
I0825 19:05:49.564351  600478 node_info.go:387] ignored_list is []
I0825 19:05:49.564378  600478 node_info.go:391] ignoredDevicesList is []
I0825 19:05:49.564439  600478 node_info.go:387] ignored_list is []
I0825 19:05:49.564459  600478 node_info.go:391] ignoredDevicesList is []
I0825 19:05:49.564599  600478 event_handlers.go:471] Added pod <c1/p1> into cache.
I0825 19:05:49.564629  600478 event_handlers.go:916] Add PodGroup(pg1) into cache, spec(v1beta1.PodGroupSpec{MinMember:1, MinTaskMember:map[string]int32(nil), Queue:"q1", PriorityClassName:"", MinResources:(*v1.ResourceList)(nil), NetworkTopology:(*v1beta1.NetworkTopologySpec)(0x20656d685a70), SubGroupPolicy:[]v1beta1.SubGroupPolicySpec(nil)})
I0825 19:05:49.564686  600478 event_handlers.go:1009] Add Queue(q1) into cache, spec(v1beta1.QueueSpec{Weight:1, Capability:v1.ResourceList(nil), Reclaimable:(*bool)(nil), ExtendClusters:[]v1beta1.Cluster(nil), Guarantee:v1beta1.Guarantee{Resource:v1.ResourceList(nil)}, Affinity:(*v1beta1.Affinity)(nil), Type:"", Parent:"", Deserved:v1.ResourceList(nil), Priority:0, DequeueStrategy:""})
I0825 19:05:49.564774  600478 node_info.go:191] set numa scheduler info of node s0-n1 to nil
I0825 19:05:49.564797  600478 node_info.go:191] set numa scheduler info of node s1-n2 to nil
I0825 19:05:49.564860  600478 node_info.go:387] ignored_list is []
I0825 19:05:49.564881  600478 node_info.go:391] ignoredDevicesList is []
I0825 19:05:49.564903  600478 node_info.go:248] imageStates is map[]
I0825 19:05:49.564957  600478 node_info.go:387] ignored_list is []
I0825 19:05:49.564974  600478 node_info.go:391] ignoredDevicesList is []
I0825 19:05:49.564993  600478 node_info.go:248] imageStates is map[]
I0825 19:05:49.565067  600478 cache.go:1548] The priority of job <c1/pg1> is </0>
I0825 19:05:49.565193  600478 cache.go:1584] "SnapShot for scheduling" jobNum=1 QueueNum=1 NodeNum=2
I0825 19:05:49.565258  600478 cache.go:1587] "HyperNode snapShot for scheduling" tiers={"0":{"s0":{},"s1":{}}} realNodesSet={"s0":{"s0-n1":{}},"s1":{"s1-n2":{}}} hyperNodesReadyToSchedule=true
I0825 19:05:49.565301  600478 session.go:1046] Start adjusting jobs' network topology spec according to hyperNodeTierNameMap map[]
I0825 19:05:49.565331  600478 session.go:1089] Finish adjusting jobs' network topology spec according to hyperNodeTierNameMap map[]
I0825 19:05:49.565371  600478 session.go:264] "hyperNode in session" name="s1" tier=0 parent="<cluster-top-hypernode>" children={}
I0825 19:05:49.565394  600478 session.go:264] "hyperNode in session" name="s0" tier=0 parent="<cluster-top-hypernode>" children={}
I0825 19:05:49.565416  600478 session.go:264] "hyperNode in session" name="<cluster-top-hypernode>" tier=1 parent="" children={"s0":{},"s1":{}}
I0825 19:05:49.565446  600478 session.go:280] Open Session f1885ab9-ed57-4234-b8e2-4396d63f63c7 with <1> Job and <1> Queues. HyperNodesReadyToSchedule: true
I0825 19:05:49.565638  600478 factory.go:60] Register preBinder predicates successfully
E0825 19:05:49.565667  600478 xpu_domain_aware.go:91] XPU-DOMAIN-AWARE-DEBUG: OnSessionOpen called, devicesPerTask=8
I0825 19:05:49.565707  600478 job_info.go:1091] job pg1/c1 actual: map[:1], ji.TaskMinAvailable: map[]
I0825 19:05:49.565754  600478 allocate.go:123] Enter Allocate ...
I0825 19:05:49.565775  600478 job_info.go:1091] job pg1/c1 actual: map[:1], ji.TaskMinAvailable: map[]
I0825 19:05:49.565827  600478 allocate.go:193] Added Job <c1/pg1> into Queue <q1>
I0825 19:05:49.565851  600478 allocate.go:138] Try to allocate resource to 1 Queues
I0825 19:05:49.565885  600478 allocate.go:311] "Try to allocate resource for job contains hard topology or subjob policy" queue="q1" job="c1/pg1" allocatedHyperNode="" subJobNum=1
I0825 19:05:49.565921  600478 allocate.go:393] "Try to allocate resource for job in hyperNode" job="c1/pg1" hyperNode="<cluster-top-hypernode>"
I0825 19:05:49.565958  600478 allocate.go:474] "Try to allocate resource for subJob" job="c1/pg1" subJob="c1/pg1" allocatedHyperNode="" nominatedHyperNode="" taskNum=1
E0825 19:05:49.565987  600478 xpu_domain_aware.go:121] XPU-DOMAIN-AWARE-DEBUG: REJECT hyperNode=s1 subJob=c1/pg1 devicesPerTask=8
E0825 19:05:49.566013  600478 xpu_domain_aware.go:121] XPU-DOMAIN-AWARE-DEBUG: REJECT hyperNode=s0 subJob=c1/pg1 devicesPerTask=8
E0825 19:05:49.566031  600478 xpu_domain_aware.go:125] XPU-DOMAIN-AWARE-DEBUG: gradient fn done, survivors=0
I0825 19:05:49.566098  600478 allocate.go:506] "Find solution for subJob fail" subJob="c1/pg1" gradient=0
I0825 19:05:49.566139  600478 allocate.go:537] "Cannot find any solution for subJob" subJob="c1/pg1"
I0825 19:05:49.566180  600478 allocate.go:434] "Find solution for job fail" job="c1/pg1" gradient=0
I0825 19:05:49.566212  600478 allocate.go:461] "Cannot find any solution for job" job="c1/pg1"
I0825 19:05:49.566246  600478 allocate.go:301] Can not find jobs for queue q1.
I0825 19:05:49.566266  600478 allocate.go:140] Leaving Allocate ...
I0825 19:05:49.566414  600478 cache.go:1103] Updating pod condition for c1/p1 to (PodScheduled==False)
I0825 19:05:49.917733  600478 cache.go:1103] Updating pod condition for c1/p1 to (PodScheduled==False)
I0825 19:05:49.917829  600478 session.go:578] Close Session f1885ab9-ed57-4234-b8e2-4396d63f63c7
--- PASS: TestXPUDomainAware_NoSatisfyingDomainAnywhereBlocksAllocation (0.36s)
PASS
I0825 19:05:49.917929  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.917978  600478 watch.go:218] "Stopping fake watcher"
I0825 19:05:49.918015  600478 watch.go:218] "Stopping fake watcher"
ok  	volcano.sh/volcano/pkg/scheduler/plugins/xpu-domain-aware/working	0.792s
```

## What this proves

1. The hook point works. `HyperNodeGradientForSubJobFn` can carry domain data before HyperNode/Node selection with no new scheduler machinery — it composes with `network-topology-aware` and, per PR #5349's pattern, would compose with `group-topology-affinity` the same way.
2. The interface is genuinely vendor-neutral, not just in principle.
3. It changes real scheduling outcomes, verified against actual Volcano plugin code — not a standalone toy harness.

## Integration detail worth flagging

`HyperNodeGradientForSubJobFn` is called with the **search root** (`ClusterTopHyperNode`), not individual candidates. A gradient plugin has to expand `ssn.HyperNodes` / `ssn.RealNodesList` itself and filter the expansion — mirroring what `network-topology-aware`'s BFS-based `hyperNodeGradientFn` already does internally. This wasn't obvious from the design docs and only surfaced while wiring this PoC against the real framework code.

## Explicitly out of scope

This PoC proves the mechanism, not the full project. Left out:

- Concrete device ID selection (`AllocateFunc`/`DeAllocateFunc` wiring)
- Reservation and gang-level rollback
- Cross-node fabric domains (Case 3, e.g. GB200 NVL72)
- Real Device Plugin / DRA `ResourceSlice` ingestion (mock provider only)

These are the actual project scope (Expected Outcomes 1–3 in the issue), not something a PoC of this size should attempt.