# ghcr-pull

Creates the `ghcr-pull` image pull Secret in each `mycloud-<env>` namespace from Vault 

The token is a classic PAT with `read:packages` only. Rotate it before it expires