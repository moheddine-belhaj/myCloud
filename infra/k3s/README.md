# k3s host configuration (cp1)

Files here are applied **by hand** on the control plane node (not by Argo CD).
They contain no secrets.

| File in repo | Location on cp1 |
|---|---|
| `audit-policy.yaml` | `/var/lib/rancher/k3s/server/audit.yaml` |
| `config.yaml.d/10-audit.yaml` | `/etc/rancher/k3s/config.yaml.d/10-audit.yaml` |

## Apply

```bash
scp infra/k3s/audit-policy.yaml infra/k3s/config.yaml.d/10-audit.yaml controlPlane:/tmp/
ssh controlPlane '
  sudo mkdir -p /var/lib/rancher/k3s/server/logs /etc/rancher/k3s/config.yaml.d &&
  sudo install -m 600 /tmp/audit-policy.yaml /var/lib/rancher/k3s/server/audit.yaml &&
  sudo install -m 600 /tmp/10-audit.yaml /etc/rancher/k3s/config.yaml.d/10-audit.yaml &&
  sudo systemctl restart k3s && sudo systemctl is-active k3s'
```

Rollback: delete `/etc/rancher/k3s/config.yaml.d/10-audit.yaml` and restart k3s.

## Query the audit log (on cp1)

Who touched a Secret:
```bash
sudo jq -c 'select(.objectRef.resource=="secrets")
  | {t: .requestReceivedTimestamp, user: .user.username, verb, ns: .objectRef.namespace,
     name: .objectRef.name, ip: .sourceIPs[0], code: .responseStatus.code}' \
  /var/lib/rancher/k3s/server/logs/audit.log | tail -20
```

Who ran exec / port-forward:
```bash
sudo jq -c 'select(.objectRef.subresource=="exec" or .objectRef.subresource=="portforward")
  | {t: .requestReceivedTimestamp, user: .user.username, ns: .objectRef.namespace, pod: .objectRef.name, ip: .sourceIPs[0]}' \
  /var/lib/rancher/k3s/server/logs/audit.log | tail
```

Noisiest users (to tune the policy):
```bash
sudo jq -r '.user.username' /var/lib/rancher/k3s/server/logs/audit.log | sort | uniq -c | sort -rn | head
```