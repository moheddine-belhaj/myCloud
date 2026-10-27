# 09 · Audit logging

Goal: record who reads secrets and who logs into the databases.

1. Write a k3s API audit policy (`infra/k3s/audit-policy.yaml`): `Metadata` level for Secrets, ConfigMaps, ServiceAccount tokens, TokenReviews and `pods/exec|attach|portforward|log`; skip reads and noise.
2. Enable it on `cp1` with a drop-in `/etc/rancher/k3s/config.yaml.d/10-audit.yaml` (`kube-apiserver-arg+`), log rotation 10 × 100 MB, 30 days.
3. Restart k3s on `cp1` and verify a Secret read appears with the laptop's Tailscale IP.
4. Restrict the admin kubeconfig to mode `0600`.
5. Turn on `log_connections` and `log_disconnections` in the CNPG base for all environments.
6. Verify successful and failed logins in the Postgres logs.

Result: every Secret read, `kubectl exec` and database login leaves a trace.
