# yaml-samples (test fixtures)

Copies of the YAML samples in the spec doc's "YAML samples" tab, with the
explanatory comments removed and nothing else changed. The spec doc is the
source of truth: when a sample changes there, update the copy here.

Tests use them as follows:

| Directory | Used by |
| --- | --- |
| `crd-definitions/` | `test/crds`: generated CRDs must match exactly |
| `crds/` | `test/envtest`: every instance must be admitted by the generated CRDs |
| `roles/`, `rolebindings/` | `test/chart`: rendered RBAC must match, apart from the illustrative extras |
| `namespace/`, `serviceaccounts/` | `test/chart`: rendered objects must match |

The binding samples other than `clusterrolebinding-kaddie-agent-read.yaml` and
`clusterrolebinding-kaddie-operator.yaml` follow Architecture section 6.2's
binding table: one binding per component, named after its role.
