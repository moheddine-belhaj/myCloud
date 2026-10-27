# handlers

| File | What it does |
|------|--------------|
| `main.yml` | Three handlers run only when a task changes something: **Reload systemd** (re-read unit files), **Restart vault** (seals Vault, must unseal again), **Reload vault** (SIGHUP, stays unsealed). |
