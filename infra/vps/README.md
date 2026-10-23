# VPS (OVH, Ubuntu 24.04)

Hosts the portfolio (Docker Compose + Traefik, public 80/443) and Vault
(systemd, Tailscale only). Vault is never published through Docker or Traefik.

| What | Value |
|---|---|
| Tailscale name / tag | `vps` / `tag:vault` |
| Vault URL | `https://vault.mohedine.dev:8200` (DNS only → Tailscale IP) |
| Certificate | certbot + dns-cloudflare, `/etc/letsencrypt/live/vault.mohedine.dev/` |
| Cloudflare token | `/root/.secrets/cloudflare.ini` (Zone DNS edit, mohedine.dev only) |
| Firewall | 8200 allowed only on `tailscale0` |

## Checks
```bash
dig +short vault.mohedine.dev                       # 100.x
sudo certbot renew --dry-run
sudo ufw status verbose
```