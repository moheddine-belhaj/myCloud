# 05 · Public access: Cloudflare Tunnel and Ingress

Goal: reach the app from the internet without opening any port at home.

1. Create a Cloudflare Tunnel for `mohedine.dev`; store its token as an encrypted secret.
2. Create the AppProject `platform` for shared cluster components.
3. Deploy `cloudflared` in namespace `cloudflared` (2 replicas, one per node) as an Argo CD app.
4. Route the tunnel to Traefik inside the cluster.
5. Add an Ingress to the api base; set the host per environment: `cloud-dev`, `cloud-qa` and `cloud.mohedine.dev`.
6. Protect dev and qa with Cloudflare Access; prod stays public.

Result: each environment has its own public URL, served through the tunnel with Cloudflare's WAF and DDoS protection.
