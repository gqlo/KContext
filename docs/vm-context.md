# VM alert context collection

**Status:** design (not yet implemented)

When a **new firing** alert is VM- or storage-related, KContext asynchronously collects surrounding cluster context, stores a snapshot in Redis, and shows it on the alert detail page.

This is the first slice of the broader “context bundling” goal in the [README](../README.md).

## Goals

- Capture a useful triage snapshot at **fire time** (not only when someone opens the UI later).
- Cover both **per-VMI** alerts and **node-scoped** VM storage alerts (no single VMI in labels).
- Collect hypervisor-side signals (virt-launcher logs, QEMU monitor), guest-side signals when the guest agent allows it, PVC/volume object context, and **node-level** signals for node-scoped (and optionally multi-VM) storage alerts.
- Reuse existing `oc` auth (`runOcLoggedIn` / SA token) — no new cluster client library.
- Deduplicate collection so Alertmanager poll loops do not re-fetch while an alert stays firing.
- Fail soft: missing RBAC, deleted VMI, no guest agent, or `oc` errors are recorded on the context object; the alert and detail page still work.

## Non-goals

- AI summarization of the collected text.
- Attaching context to Slack notifications.
- Password-based SSH into the guest (`virtctl ssh` / `helpers/log-vm` style).
- Long-lived QEMU event streams (`virsh qemu-monitor-event --loop`).
- Scraping Prometheus for historical metric series (object describe + QMP snapshot only in this design).
- Always-on host journal collection for every per-VMI alert (node journals are **Node-profile / opt-in** only).

## Flow

```text
  Poll or Webhook
        |
        v
  AlertStore Save / SyncPolled
        |
        |  new firing + NeedsVMContext
        v
  TryEnqueue context ----> Async collector
                                |
                    +-----------+-----------+
                    | profile: VMI / Node / |
                    | PVC  (oc / oc exec)   |
                    +-----------+-----------+
                                |
                                v
                   Redis kcontext:vm-context:fp
                                |
                                v
                   GET /alert --> Detail panels
```

1. Alert is stored via poll ([alert-polling.md](alert-polling.md)) or webhook ([webhook-ingest.md](webhook-ingest.md)).
2. If status is `firing` and the alert needs context (`NeedsVMContext()`), enqueue collection keyed by **fingerprint**.
3. A bounded worker selects a **collection profile** (VMI vs node), runs `oc` / `oc exec` commands, and writes a JSON snapshot to Redis.
4. `GET /alert?id=…` loads the snapshot by the alert’s fingerprint and renders panels.

## Alert detection

Helpers on `StoredAlert` (planned):

| Helper | Behavior |
|--------|----------|
| `Namespace()` | Existing — `namespace`, `kubernetes_namespace`, … |
| `Pod()` | Existing — `pod`, `kubernetes_pod_name`, … |
| `NodeName()` | First non-empty among labels: `node`, `kubernetes_node`, `node_name` |
| `VMIName()` | First non-empty among labels: `name`, `vmi`, `virtualmachineinstance`, `vm` |
| `PVCName()` | First non-empty among labels: `persistentvolumeclaim`, `pvc` |
| `NeedsVMContext()` | True when any of: VMI profile matches, node profile matches, or PVC profile matches (see below) |

### Profiles

| Profile | When | Collect |
|---------|------|---------|
| **VMI** | `Namespace()` set and (`VMIName()` non-empty **or** `Pod()` starts with `virt-launcher-` **or** `alertname` matches `(?i)^(VMI\|KubeVirt\|VirtualMachine)`) | Full per-VMI bundle (below) |
| **Node** | `NodeName()` set and alert looks VM/storage related (e.g. `alertname` matches `(?i)Node.*VM\|VM.*Node` **or** annotation/summary mentions VM storage) — includes `NodeVMStorageLatencyWidespread` | Node describe, VMIs on node (capped), sample launcher logs, node/PVC events, **CSI/node-plugin logs**, optional host journal |
| **PVC** | `PVCName()` + `Namespace()` set (e.g. `PVCBlockLatencyHigh`, `PVCNFSLatencyHigh`) | PVC/PV describe + events; if `Pod()` is virt-launcher or VMI resolvable, also attach VMI bundle |

An alert may match more than one profile (e.g. VMI + PVC labels). The worker merges collectors; each section fails independently.

### Examples

| Labels | Profile(s) |
|--------|------------|
| `namespace=demo`, `name=fio-vm`, `alertname=VMIStorageWriteLatencyHigh` | VMI |
| `namespace=demo`, `name=fio-vm`, `persistentvolumeclaim=fio-root`, `disk=rootdisk` | VMI + PVC |
| `node=worker-1`, `alertname=NodeVMStorageLatencyWidespread` | Node |
| `namespace=demo`, `persistentvolumeclaim=data-pvc`, `pod=virt-launcher-…`, `alertname=PVCBlockLatencyHigh` | PVC (+ VMI if resolvable) |
| `namespace=openshift-monitoring`, `alertname=TargetDown` | None |

## Collect once per firing episode

Alert list entries get a new `id` on every store; Alertmanager **fingerprint** is stable for the same label set.

| Concern | Approach |
|---------|----------|
| Dedup while firing | Redis key `kcontext:vm-context:{fingerprint}` — claim with `SET NX` (pending placeholder) or an inflight set so poll does not re-enqueue |
| Re-fire after resolve | When the fingerprint leaves the active set (resolved), delete or expire the context claim so a later firing can collect again |
| Webhook + poll | Same fingerprint key — either path can claim; the other is a no-op |
| Concurrency | Small worker queue started from `run.go` (bounded goroutines) |
| Time budget | Overall context timeout ~45–60s; per-command timeouts; skip remaining steps on deadline |

Only **firing** alerts enqueue. Resolved / suppressed writes do not collect.

## Collector

Planned module: `vmcontext.go`, using `runOcLoggedIn` (and `oc exec` into virt-launcher `compute`).

### VMI profile steps

1. Resolve `namespace` + VMI name (`VMIName()`; if empty and pod is `virt-launcher-*`, read pod labels `vm.kubevirt.io/name` / `kubevirt.io/domain`).
2. Resolve virt-launcher pod:
   - Use `Pod()` when it starts with `virt-launcher-`; else
   - `oc get pod -n <ns> -l kubevirt.io=virt-launcher` and select by domain / vmi-name label (same idea as [vstorm `kubevirt-dirty-rate.sh`](../../vstorm/monitoring/scripts/kubevirt-dirty-rate.sh)).
3. Collect in parallel where safe (respect overall deadline):

| Artifact | How |
|----------|-----|
| VMI describe | `oc describe vmi <vmi> -n <ns>` |
| virt-launcher logs | `oc logs <pod> -n <ns> -c compute --tail=<N>`; retry without `-c` if compute missing |
| Guest OS logs | Via QEMU guest agent (no SSH passwords): `oc exec -n <ns> <pod> -c compute -- virsh qemu-agent-command <domain> --pretty '…guest-exec journalctl…'` (or equivalent guest-exec + guest-exec-status). Default command: `journalctl -n <N> --no-pager`. Skip with recorded error if GA not connected / exec denied |
| QEMU monitor dump | `oc exec … virsh qemu-monitor-command <domain> …` for a fixed set of read-only queries, concatenated: `query-status`, `query-block`, `query-blockstats`, `query-iothreads` (omit any that error). Optionally also `virsh dominfo` / `domblklist` |
| PVC / volume | From alert `PVCName()` / `disk`, or from VMI volume status / PVC map: `oc describe pvc <pvc> -n <ns>`, `oc get pv <pv> -o yaml` (or describe) when bound; `oc get events -n <ns> --field-selector involvedObject.name=<pvc>` |

Domain name for virsh: `virsh list --name` inside the pod (first domain), matching the dirty-rate helper pattern.

### Node profile steps

For alerts with `node=<name>` and no VMI (e.g. `NodeVMStorageLatencyWidespread`):

| Artifact | How |
|----------|-----|
| Node describe | `oc describe node <node>` |
| VMIs on node | `oc get vmi -A -o json` filtered by `status.nodeName` (or equivalent); cap list length (`VM_CONTEXT_NODE_VMI_LIMIT`, default `20`) |
| Sample launcher logs | For up to `VM_CONTEXT_NODE_LOG_SAMPLE` (default `3`) Running VMIs on that node: virt-launcher `--tail` (shorter than full VMI profile, e.g. 100 lines) |
| Node events | `oc get events -A --field-selector involvedObject.name=<node>,involvedObject.kind=Node` (or namespace-scoped fallback) |
| Storage hint | If PVC names appear on listed VMIs, describe up to N distinct PVCs |
| CSI / node-plugin logs | See [Node-level logs](#node-level-logs) (tier 1 — default on) |
| Host journal / dmesg | See [Node-level logs](#node-level-logs) (tier 2 — opt-in) |

Do **not** run guest-exec / full QMP dumps for every VMI on the node in v1 (too slow); only the sampled launcher logs + node/PVC describes + node-level log tiers below.

### Node-level logs

For node-wide storage alerts, host/CSI signals often matter more than another virt-launcher sample. Collect in two tiers.

#### Tier 1 (default) — CSI / storage DaemonSet pods on the node

Prefer pod logs (RBAC-friendly: `pods` list + `pods/log`) over chrooting the host:

1. List pods scheduled on the node:  
   `oc get pods -A --field-selector spec.nodeName=<node> -o json`
2. Select storage-related candidates (cap count, e.g. `VM_CONTEXT_NODE_CSI_LOG_LIMIT`, default `5`):
   - Namespace hints: `openshift-cluster-csi-drivers`, `openshift-storage`, `kube-system` (in-tree/CSI remnants)
   - Name / label hints: `csi-`, `driver`, `ceph`, `odf`, `lvm`, `nfs`, `multipath` (best-effort match list; miss → skip, do not fail the snapshot)
3. For each selected pod: `oc logs -n <ns> <pod> --tail=<N> --all-containers=true` (or primary container if `--all-containers` is too large)

Store concatenated output in `node_csi_logs` (with a short header per pod: `--- ns/pod ---`).

#### Tier 2 (opt-in) — host journal / kernel

Kubelet, CRI, and kernel I/O messages are high value for attach/mount failures and disk errors, but need node debug access:

```bash
oc debug node/<node> -- chroot /host journalctl -u kubelet -n <N> --no-pager
oc debug node/<node> -- chroot /host journalctl -u crio -n <N> --no-pager
oc debug node/<node> -- chroot /host journalctl -k -n <N> --no-pager   # or dmesg
```

| Gate | Behavior |
|------|----------|
| `VM_CONTEXT_NODE_JOURNAL=true` | Enable tier 2; default `false` |
| Privileges | Requires ability to create debug pods / privileged access — beyond normal SA `pods/log`. If debug fails, record in `error` and continue |
| Timeout | Count against `VM_CONTEXT_TIMEOUT`; skip remaining journal commands on deadline |

Store in `node_kubelet_log`, `node_cri_log`, `node_kernel_log` (empty when disabled or failed).

#### When to skip node logs

- Pure per-VMI alerts (`VMIStorageWriteLatencyHigh`) — VMI + launcher + QMP + PVC is enough; do **not** pull node journals unless the same fingerprint also matches the Node profile.
- No CSI pods found on the node — leave `node_csi_logs` empty; still keep describe/events/VMI list.

### PVC profile steps

When `persistentvolumeclaim` (or `pvc`) is on the alert:

| Artifact | How |
|----------|-----|
| PVC describe | `oc describe pvc <pvc> -n <ns>` |
| Bound PV | From PVC `.spec.volumeName` → `oc describe pv <pv>` |
| PVC events | Field-selector on the PVC name |
| Optional VMI attach | If `pod` is virt-launcher or `name` is a VMI, run the VMI profile as well |

### Snapshot schema

```json
{
  "fingerprint": "…",
  "collected_at": "2026-07-23T13:00:00Z",
  "profile": ["vmi", "pvc"],
  "namespace": "demo",
  "vmi_name": "fio-vm",
  "pod_name": "virt-launcher-fio-vm-xxxxx",
  "node_name": "",
  "pvc_name": "fio-root",
  "describe_vmi": "Name: …",
  "launcher_log": "…",
  "guest_log": "…",
  "qemu_monitor": "…",
  "describe_pvc": "…",
  "describe_pv": "…",
  "pvc_events": "…",
  "describe_node": "",
  "node_vmis": "",
  "node_sample_logs": "",
  "node_events": "",
  "node_csi_logs": "",
  "node_kubelet_log": "",
  "node_cri_log": "",
  "node_kernel_log": "",
  "error": ""
}
```

| Field | Meaning |
|-------|---------|
| `profile` | Which collectors ran |
| Text bodies | Raw command stdout (empty if skipped/failed) |
| `error` | Non-fatal summary of failures (newline-joined) |
| `collected_at` | When the worker finished (or wrote the error record) |

Pending claim (before worker finishes) may store a stub with empty bodies so the UI can show “collecting…”.

## Redis keys

| Key | Type | Purpose |
|-----|------|---------|
| `kcontext:vm-context:{fingerprint}` | String (JSON) | Latest context snapshot (or pending stub) for that fingerprint |

See also existing alert keys in [alert-polling.md](alert-polling.md) and [redis-migration.md](redis-migration.md). Context blobs are **optional** to migrate; dashboards still work without them.

## Detail UI

On [alert detail](endpoints.md) (`GET /alert?id=…`):

- If the alert has a fingerprint, load `kcontext:vm-context:{fingerprint}`.
- When present, show panels for each non-empty artifact (VMI describe, virt-launcher logs, guest logs, QEMU monitor, PVC/PV/events, node describe / VMI list / sample logs, CSI node logs, optional host journal panels).
- When pending: short “Collecting context…” note.
- When `error` is set: show the error; still show any partial output.
- Existing overview / labels / annotations panels unchanged.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `VM_CONTEXT_ENABLED` | `true` | Set `false` to disable enqueue and collection |
| `VM_CONTEXT_LOG_TAIL` | `500` | `--tail` for per-VMI virt-launcher logs |
| `VM_CONTEXT_GUEST_LOG_LINES` | `200` | `journalctl -n` inside guest via guest agent |
| `VM_CONTEXT_NODE_VMI_LIMIT` | `20` | Max VMIs listed for node-scoped alerts |
| `VM_CONTEXT_NODE_LOG_SAMPLE` | `3` | Max VMIs to sample launcher logs on node alerts |
| `VM_CONTEXT_NODE_CSI_LOG_LIMIT` | `5` | Max CSI/storage pods to pull logs from on the node |
| `VM_CONTEXT_NODE_CSI_LOG_TAIL` | `200` | `--tail` for CSI/node-plugin pod logs |
| `VM_CONTEXT_NODE_JOURNAL` | `false` | Enable tier-2 host journal via `oc debug node` (kubelet/crio/kernel) |
| `VM_CONTEXT_NODE_JOURNAL_LINES` | `200` | Line count for each host journal query |
| `VM_CONTEXT_TIMEOUT` | `60s` | Overall collector deadline per fingerprint |

Documented alongside other env vars in [configuration.md](configuration.md) once implemented.

## RBAC

The `kcontext` ServiceAccount needs cluster read access beyond today’s monitoring / CSV meta roles:

| API | Resources | Verbs |
|-----|-----------|-------|
| `""` | `pods` | `get`, `list` |
| `""` | `pods/log` | `get` |
| `""` | `pods/exec` | `create` (for `oc exec` into virt-launcher) |
| `""` | `persistentvolumeclaims` | `get`, `list` |
| `""` | `persistentvolumes` | `get`, `list` |
| `""` | `events` | `list` |
| `""` | `nodes` | `get`, `list` (node describe; may already exist via cluster-meta role) |
| `kubevirt.io` | `virtualmachineinstances` | `get`, `list` |

Planned manifests under `deploy/openshift/` (ClusterRole + ClusterRoleBinding), applied by the same path as [alert-polling.md](alert-polling.md) token setup (`ensureOpenShiftRBAC` / `create-local-token.sh`).

**Guest agent note:** guest-exec must be allowed by the guest’s qemu-guest-agent policy. If exec is disabled or the agent is down, `guest_log` stays empty and `error` records the reason — no SSH fallback.

**Node journal note:** tier-1 CSI pod logs use the same `pods` / `pods/log` verbs. Tier-2 `oc debug node` needs privileges beyond this table (often cluster-admin or a dedicated debug Role); keep it behind `VM_CONTEXT_NODE_JOURNAL` and fail soft.

Without these permissions, collection writes an `error` on the snapshot; alerts continue to ingest normally.

## Testing (planned)

- **Detection:** VMI labels, PVC labels, node-only alert (`NodeVMStorageLatencyWidespread`), unrelated alert.
- **Enqueue:** second firing poll with the same fingerprint does not start a second collection; after resolve, re-fire collects again.
- **Collector:** injectable `oc` runner — VMI happy path; guest-agent missing; QMP partial failure; PVC-only; node profile with VMI list cap + CSI pod log selection; journal tier skipped when disabled / debug fails; compute container fallback.

## Implementation map

| Area | Location |
|------|----------|
| Detection helpers | `store.go` |
| Collector + Redis + worker | `vmcontext.go` (new) |
| Enqueue after store | `store.go` (`Save` / `SyncPolled`) and/or webhook + poll call sites |
| Worker start | `run.go` |
| Detail panels | `details.go` |
| RBAC | `deploy/openshift/` |
| Tests | `tests/` |

## Related

| Doc | Topic |
|-----|-------|
| [alert-flow.md](alert-flow.md) | Poll vs webhook ingest |
| [alert-polling.md](alert-polling.md) | Poll dedup and Redis alert keys |
| [webhook-ingest.md](webhook-ingest.md) | Webhook receiver |
| [endpoints.md](endpoints.md) | `/alert` route |
| [configuration.md](configuration.md) | Environment variables |
| [architecture.md](architecture.md) | How KContext works today |
| [redis-migration.md](redis-migration.md) | Copying Redis data between instances |
