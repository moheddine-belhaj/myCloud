# 00 · Home lab and k3s cluster

Goal: a private Kubernetes cluster at home that can only be administered over Tailscale.

1. Prepare two Fujitsu Esprimo Q958 mini-PCs: `cp1` (control plane) and `worker1` (worker).
2. Install k3s on `cp1` as the server, with Traefik as the built-in ingress controller.
3. Join `worker1` to the cluster as an agent.
4. Install Tailscale on both nodes and on the laptop.
5. Point the laptop's kubeconfig at the control plane's Tailscale IP.
6. Keep the home network closed: no public IPv4, no open ports (DS-Lite).

Result: a 2-node k3s cluster, managed with `kubectl` from the laptop over Tailscale.
