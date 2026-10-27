# infra

Infrastructure as code for MyCloud's supporting servers. Today this folder holds one thing: an
**Ansible** setup that installs and configures **HashiCorp Vault** on the VPS.

Vault is reachable **only over Tailscale** (never on the public IP) at `https://<VAULT_DOMAIN>:8200`.
The name resolves (Cloudflare, DNS only) to the VPS's Tailscale IP and has a real Let's Encrypt certificate.

## Layout

```
infra/
└── ansible/
    ├── ansible.cfg          Ansible settings for this folder
    ├── inventory.yml        Which hosts to manage
    ├── requirements.yml     Ansible collections to install
    ├── vault.yml            The playbook (entry point)
    └── roles/vault/
        ├── defaults/main.yml    Tunable variables
        ├── handlers/main.yml    Reload / restart actions
        ├── tasks/main.yml       The steps, in order
        └── templates/           Files rendered onto the VPS
```

## What each file does

### `ansible/` (top level)

| File | Role |
|------|------|
| `ansible.cfg` | Points Ansible at `inventory.yml` and `roles/`, prints results as YAML, disables `.retry` files, enables SSH pipelining (faster). Only picked up when you run commands **from `infra/ansible/`**. |
| `inventory.yml` | Defines the group `vault` with one host, `vps`. You fill in `ansible_host` (public IP or an `~/.ssh/config` alias) and `ansible_user`. No secrets: SSH keys stay in `~/.ssh`, the sudo password is asked at run time (`-K`). |
| `requirements.yml` | Extra collections needed: `community.general` (provides the `ufw` firewall module). |
| `vault.yml` | The playbook. Runs the `vault` role on the `vault` group with root privileges (`become: true`). |

### `ansible/roles/vault/`

| File | Role |
|------|------|
| `defaults/main.yml` | Every setting in one place: domain (`<VAULT_DOMAIN>`), network interface (`tailscale0`), ports (8200 API, 8201 cluster), data/TLS/audit directories, Raft node id, Let's Encrypt cert path, audit log retention (30 days). Override in `inventory.yml` if needed. |
| `handlers/main.yml` | Actions triggered by config changes: **Reload systemd**, **Restart vault**, **Reload vault**. A *restart seals Vault* (you must unseal again); a *reload* (SIGHUP) keeps it unsealed. |
| `tasks/main.yml` | The installation, in 7 sections (below). |

### `ansible/roles/vault/templates/`

Jinja2 templates rendered onto the VPS. Each starts with an "Ansible managed" header, so don't edit the results by hand.

| Template | Installed at | What it does |
|----------|--------------|--------------|
| `vault.hcl.j2` | `/etc/vault.d/vault.hcl` | Vault server config: UI on, integrated **Raft** storage in the data dir, TLS listener bound to the **Tailscale IP only** (TLS 1.2+), API and cluster addresses. |
| `vault-override.conf.j2` | `/etc/systemd/system/vault.service.d/override.conf` | Systemd drop-in: start after Tailscale, keep retrying at boot until the Tailscale IP exists, restart on failure. |
| `certbot-deploy-hook.sh.j2` | `/etc/letsencrypt/renewal-hooks/deploy/vault.sh` | Runs after every certbot renewal: copies `fullchain.pem`/`privkey.pem` to Vault's TLS dir (readable by user `vault`) and reloads Vault (stays unsealed). Ignores other certificates. |
| `logrotate.j2` | `/etc/logrotate.d/vault` | Rotates the audit logs daily, keeps 30 days, compresses, then reloads Vault so it reopens the log file. |
| `vault-env.sh.j2` | `/etc/profile.d/vault.sh` | Sets `VAULT_ADDR` for shells on the VPS so the `vault` CLI talks to the local server by its real name (valid TLS). |

### What `tasks/main.yml` does, step by step

1. **Preconditions**: fails early if `tailscale0` has no IPv4 or the Let's Encrypt certificate is missing; remembers the Tailscale IP.
2. **Install**: removes a hand-added HashiCorp apt repo (it conflicts with the managed one), adds the signed HashiCorp repo, installs `vault`, then **holds** the package so an `apt upgrade` can't restart (and seal) Vault.
3. **Directories**: data (`vault:vault`, 0700), TLS (`root:vault`, 0750), audit logs (`vault:vault`, 0750).
4. **TLS**: installs the certbot deploy hook and, if Vault's copy is missing or differs from the Let's Encrypt one, runs the hook once to install it.
5. **Config and service**: renders `vault.hcl`, the systemd drop-in, logrotate and `VAULT_ADDR`; changes notify the handlers.
6. **Firewall**: UFW allows TCP 8200 **only on `tailscale0`**.
7. **Start**: flushes handlers, enables and starts the service, waits for the health endpoint (sealed or uninitialised is fine), prints version/initialized/sealed.

The role is **idempotent**: re-running it changes nothing unless something drifted or a variable changed.

## Not automated (do by hand)

- **Tailscale** installed and up on the VPS (`sudo tailscale up`).
- **Certbot certificate** for `<VAULT_DOMAIN>` (step 3a), so `/etc/letsencrypt/live/<VAULT_DOMAIN>/fullchain.pem` exists.
- **`vault operator init` and `vault operator unseal`**: these print the unseal keys and root token. Store them safely, never in git. You must unseal again after every restart of the service or reboot.

## How to apply





Prerequisites on your workstation: Ansible installed, SSH access to the VPS, a sudo-capable user there, and your workstation on the same tailnet.

```bash
# Ansible (the full package includes community.general, used for the ufw module)
sudo apt-get install -y pipx
pipx install --include-deps ansible
pipx ensurepath && exec $SHELL
ansible --version

# Vault CLI (same HashiCorp repository Ansible will add on the VPS)
wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" \
  | sudo tee /etc/apt/sources.list.d/hashicorp.list
sudo apt-get update && sudo apt-get install -y vault
vault version
echo 'export VAULT_ADDR=https://<VAULT_DOMAIN>:8200' >> ~/.bashrc && source ~/.bashrc # set your Vault domain

```

```bash
cd infra/ansible

# 1. Edit inventory.yml: set ansible_host and ansible_user for "vps"

# 2. Install the needed collection (once)
ansible-galaxy collection install -r requirements.yml

# 3. Check connectivity
ansible vault -m ping -K

# 4. Dry run: shows what would change, changes nothing
ansible-playbook vault.yml -K --check --diff

# 5. Apply (-K asks for the sudo password)
ansible-playbook vault.yml -K
```

Note: `--check` can fail on tasks that depend on earlier changes (for example the health check before Vault is installed). That is expected on a first run.

After the first apply, initialise and unseal Vault on the VPS:

```bash
vault operator init          # save the 5 unseal keys + root token offline
vault operator unseal        # repeat with 3 different keys
```

## How to test

Run these after applying. `ansible-playbook` already ends with a health check and prints
`initialized=... sealed=...`; the checks below verify each piece independently.

**1. Idempotence**: a second run must report `changed=0`.
```bash
ansible-playbook vault.yml -K          # look for: changed=0 failed=0
```

**2. Service and package** (on the VPS)
```bash
systemctl is-enabled vault && systemctl is-active vault   # enabled / active
apt-mark showhold | grep vault                            # vault is held
vault version
```

**3. Listening only on Tailscale** (on the VPS)
```bash
ss -tlnp | grep -E ':(8200|8201)'      # must show the 100.x.y.z address, NOT 0.0.0.0
sudo ufw status | grep 8200            # ALLOW IN on tailscale0
```

**4. Not reachable publicly** (from a machine **off** the tailnet): this must time out.
```bash
curl -m 5 https://<VPS_PUBLIC_IP>:8200/v1/sys/health
```

**5. Reachable over Tailscale with valid TLS** (from your workstation, on the tailnet)
```bash
curl -s https://<VAULT_DOMAIN>:8200/v1/sys/health | jq
# no certificate warning (no -k needed); fields: initialized, sealed, version
```

**6. Certificate**
```bash
sudo ls -l /opt/vault/tls                           # vault.crt 0640, vault.key 0640 (root:vault)
sudo certbot renew --dry-run                        # succeeds
sudo RENEWED_LINEAGE=/etc/letsencrypt/live/<VAULT_DOMAIN> \
  /etc/letsencrypt/renewal-hooks/deploy/vault.sh    # runs; Vault stays unsealed
```

**7. Audit log rotation** (after enabling an audit device, e.g. `vault audit enable file file_path=/var/log/vault/audit.log`)
```bash
sudo logrotate -d /etc/logrotate.d/vault            # debug mode: shows what it would do
```

**8. Unsealed after init**
```bash
vault status                                         # Initialized true, Sealed false
```
