# e2e infrastructure

`make kind-up K8S=<1.28|latest>` creates a kind cluster for the e2e tests with:

- the API server's audit log at `/var/log/kubernetes/audit/audit.log` in the
  control-plane container, written with `audit-policy.yaml` (base §9.3's
  suggested policy);
- a mock OIDC issuer (`test/mocks/oidc`) running as a container on the
  `kind` Docker network under the alias `kaddie-oidc`, trusted by the API
  server, with a host port the tests use to mint tokens. Its details are in
  `.generated/<cluster>/oidc.json`.

## Findings from the OIDC spike (implementation plan T0.3)

- **1.28** has no structured authentication config, so `kind-1.28.yaml` sets
  the `--oidc-*` flags: issuer `https://kaddie-oidc:8443`, client ID
  `kaddie-ui`, username claim `email` with prefix `oidc:`, groups claim
  `groups` with prefix `oidc:`, and `--oidc-ca-file` for the issuer's CA.
  kubeadm v1beta3 takes `extraArgs` as a map.
- **1.34 and later** (the `latest` config runs 1.37) use
  `--authentication-config` with an `AuthenticationConfiguration` of
  `apiserver.config.k8s.io/v1`, the CA inlined as PEM. kubeadm v1beta4 takes
  `extraArgs` as a list of name/value pairs. 1.30 to 1.33 would need `v1beta1`.
- The API server needs the issuer only when it validates a token, so the
  issuer can start after the cluster. The issuer URL must be HTTPS and must
  resolve from inside the control-plane container; a Docker network alias does
  that.
- The prefixes appear in `SelfSubjectReview`: a token for
  `alice@example.com` in group `devs` authenticates as `oidc:alice@example.com`
  with groups `oidc:devs` and `system:authenticated` (`TestOIDC_Spike`).
- An admin kubeconfig with client certificates ignores `--token`: client
  certificates win. Test with a kubeconfig whose user holds only the token.
- kind v0.33 boots `kindest/node:v1.28.15`, the last 1.28 image.

What the chart docs must say: the API server must trust the identity provider
kaddie-ui signs users in with, using the flags on 1.28 to 1.29 or structured
authentication config from 1.30. Whatever username and group prefixes the
deployer configures are what `rbac.operators` subjects must use.
