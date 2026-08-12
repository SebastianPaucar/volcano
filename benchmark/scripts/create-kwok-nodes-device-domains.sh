#!/usr/bin/env bash
# create-kwok-nodes-device-domains.sh — Create KWOK nodes carrying a fake
# per-node device-register annotation with two intra-node device domains,
# to reproduce the "Case 1" gap in volcano-sh/volcano#5751:
#
#   A node has N free devices in total, but they are fragmented across
#   independent physical domains (e.g. two 8-NPU HCCS/NVLink domains), so a
#   single-domain gang request can silently spill across domains.
#
# This is intentionally a SIBLING to benchmark/scripts/create-kwok-nodes.sh,
# not a modification of it — it does not touch existing benchmark behavior.
#
# The annotation format written here is NOT invented: it matches
# devices.DeviceInfo / devices.UnMarshalNodeDevices exactly
# (pkg/scheduler/api/devices/device_info.go), including the CustomInfo
# "NetworkID" convention already consumed by the Ascend/HAMi device plugin
# in pkg/scheduler/api/devices/ascend/hami/device_info.go
# (hasNetworkID / selectDevicesWithTopology). Reusing this convention means
# the repro is grounded in code that already exists and is scheduled today,
# not a speculative new topology model.
#
# Usage:
#   ./scripts/create-kwok-nodes-device-domains.sh
#   DEVICES_PER_DOMAIN=8 DOMAINS_PER_NODE=2 ./scripts/create-kwok-nodes-device-domains.sh
#
# Environment variables:
#   DOMAIN_NODE_COUNT   - number of KWOK nodes to create (default: 1)
#   DEVICES_PER_DOMAIN  - devices per domain per node (default: 8)
#   DOMAINS_PER_NODE    - number of independent domains per node (default: 2)
#   DEVICE_ANNOTATION_KEY - annotation key devices are registered under
#                            (default: "huawei.com/node-register-Ascend910",
#                            matching Ascend910Prefix / the real HAMi plugin)
#   DEVICE_RESOURCE_NAME  - allocatable resource name advertised on the node
#                            (default: "huawei.com/Ascend910")

source "$(dirname "$0")/common.sh"
require_cmd kubectl

DOMAIN_NODE_COUNT="${DOMAIN_NODE_COUNT:-1}"
DEVICES_PER_DOMAIN="${DEVICES_PER_DOMAIN:-8}"
DOMAINS_PER_NODE="${DOMAINS_PER_NODE:-2}"
DEVICE_ANNOTATION_KEY="${DEVICE_ANNOTATION_KEY:-huawei.com/node-register-Ascend910}"
DEVICE_RESOURCE_NAME="${DEVICE_RESOURCE_NAME:-huawei.com/Ascend910}"

# build_device_list_json <node_idx>
# Emits a JSON array of devices.DeviceInfo objects (see device_info.go),
# split evenly across DOMAINS_PER_NODE via CustomInfo.NetworkID, exactly the
# field the existing hasNetworkID/selectDevicesWithTopology logic reads.
function build_device_list_json() {
    local node_idx=$1
    local total=$(( DEVICES_PER_DOMAIN * DOMAINS_PER_NODE ))
    local json="["
    local first=true
    for domain in $(seq 0 $((DOMAINS_PER_NODE - 1))); do
        for i in $(seq 0 $((DEVICES_PER_DOMAIN - 1))); do
            if [ "$first" = true ]; then
                first=false
            else
                json="${json},"
            fi
            local dev_id="node${node_idx}-domain${domain}-dev${i}"
            json="${json}{\"id\":\"${dev_id}\",\"index\":${i},\"count\":1,\"devmem\":32768,\"devcore\":100,\"type\":\"Ascend910\",\"health\":true,\"devicevendor\":\"Huawei\",\"custominfo\":{\"NetworkID\":${domain}}}"
        done
    done
    json="${json}]"
    echo "${json}"
}

function create_domain_node() {
    local idx=$1
    local device_json
    device_json=$(build_device_list_json "${idx}")
    # Escape double quotes for embedding as an annotation value in YAML.
    local escaped_json
    escaped_json=$(echo "${device_json}" | sed 's/"/\\"/g')

    local total_devices=$(( DEVICES_PER_DOMAIN * DOMAINS_PER_NODE ))

    cat <<EOF
apiVersion: v1
kind: Node
metadata:
  name: kwok-domain-node-${idx}
  annotations:
    node.alpha.kubernetes.io/ttl: "0"
    kwok.x-k8s.io/node: fake
    ${DEVICE_ANNOTATION_KEY}: "${escaped_json}"
  labels:
    beta.kubernetes.io/arch: amd64
    beta.kubernetes.io/os: linux
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: kwok-domain-node-${idx}
    kubernetes.io/os: linux
    kubernetes.io/role: agent
    node-role.kubernetes.io/agent: ""
    type: kwok
spec:
  taints:
    - key: kwok.x-k8s.io/node
      value: fake
      effect: NoSchedule
status:
  allocatable:
    cpu: "${CPU_PER_NODE}"
    memory: "${MEMORY_PER_NODE}"
    pods: "110"
    ${DEVICE_RESOURCE_NAME}: "${total_devices}"
  capacity:
    cpu: "${CPU_PER_NODE}"
    memory: "${MEMORY_PER_NODE}"
    pods: "110"
    ${DEVICE_RESOURCE_NAME}: "${total_devices}"
  conditions:
    - type: Ready
      status: "True"
      reason: KubeletReady
      message: "kubelet is posting ready status"
      lastHeartbeatTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      lastTransitionTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  phase: Running
EOF
}

function main() {
    log_info "Creating ${DOMAIN_NODE_COUNT} KWOK node(s), each with ${DOMAINS_PER_NODE} domains x ${DEVICES_PER_DOMAIN} devices"
    log_info "Device annotation key: ${DEVICE_ANNOTATION_KEY}"

    for i in $(seq 0 $((DOMAIN_NODE_COUNT - 1))); do
        create_domain_node "${i}"
        echo "---"
    done | kubectl apply -f -

    log_info "Waiting for domain nodes to be ready..."
    kubectl wait --for=condition=Ready node -l type=kwok --timeout=120s 2>/dev/null || true

    log_info "Domain node creation complete."
    log_info "Dump one node's raw annotation to feed cmd/domain-repro, e.g.:"
    log_info "  kubectl get node kwok-domain-node-0 -o jsonpath='{.metadata.annotations.${DEVICE_ANNOTATION_KEY}}' > /tmp/node0-devices.json"
}

main
