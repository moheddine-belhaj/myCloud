# /etc/vault.d/vault.hcl on the VPS. Replace VPS_TS_IP with `tailscale ip -4`.
ui            = true
disable_mlock = true # recommended with integrated (Raft) storage

storage "raft" {
  path    = "/opt/vault/data"
  node_id = "vps"
}

# Listen on the Tailscale IP only: unreachable from the public internet.
listener "tcp" {
  address         = "VPS_TS_IP:8200"
  cluster_address = "VPS_TS_IP:8201"
  tls_cert_file   = "/opt/vault/tls/vault.crt"
  tls_key_file    = "/opt/vault/tls/vault.key"
  tls_min_version = "tls12"
}

api_addr     = "https://vault.mohedine.dev:8200"
cluster_addr = "https://vault.mohedine.dev:8201"