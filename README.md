# Kaddie

Kaddie, the Agentic SRE Operator, is a set of Kubernetes components that let
an LLM investigate cluster problems and draft fixes ("proposals") without
ever being able to change anything itself. A human reviews every proposal,
dry-runs it as themselves and executes it with their own credentials.

The design lives in the spec doc (Architecture, Security model, Harness,
GitOps, UI and Session Store tabs, the YAML samples and the implementation
plan). The Architecture tab is the source of truth; this repository follows
the implementation plan phase by phase.

## Status

Phases 0 and 1 of the implementation plan: the repository, the test
environments, and the cluster contract. That is the three CRDs (`SREProposal`,
`SREConfig`, `SRETrigger`) with their CEL validation, and a Helm chart with the
namespace, quota, ServiceAccounts and all RBAC. No component runs yet; the
binaries under `cmd/` only print their version.

## Layout

| Path | What |
| --- | --- |
| `cmd/` | One binary per component: kaddie-agent, kaddie-sessions, kaddie-tools, kaddie-ui, kaddie-git-writer, kaddie-renderer |
| `pkg/apis/v1alpha1` | API types for `kaddie.org/v1alpha1`; the CRDs are generated from them |
| `charts/kaddie` | The Helm chart; `crds/` is generated, RBAC comes only from `templates/_rbac_rules.tpl` |
| `hack/` | Developer tools: CRD post-processing, the traceability checker |
| `test/envtest` | CRD and CEL tests against a real 1.28 kube-apiserver |
| `test/chart` | Render tests for the chart, GitOps on and off |
| `test/e2e` | kind tests: RBAC enumeration, quota, audit, OIDC |
| `test/fixtures/yaml-samples` | The spec's YAML samples, used as test fixtures |
| `test/mocks/oidc` | Mock OIDC issuer for e2e tests |
| `test/infra` | kind configs and scripts |
| `test/TRACEABILITY.md` | Every spec test ID, its task and its implementing test |

## Development

You need Go, Helm, GNU Make, Docker, kind and kubectl. On Windows, run make
from Git Bash. Tools such as golangci-lint and controller-gen are installed
into `./bin` at pinned versions the first time a target needs them.

```sh
make help                     # every target
make lint test                # lint and unit tests
make generate                 # regenerate deepcopy code and CRDs after editing pkg/apis
make test-envtest             # CRD/CEL tests on a 1.28 kube-apiserver
make chart-test               # helm lint plus render tests
make kind-up K8S=1.28         # kind cluster with audit logging and a mock OIDC issuer
make e2e FOCUS=T6,T43         # e2e tests, optionally narrowed to spec test IDs
make kind-down
```

## Installing

```sh
helm install kaddie charts/kaddie --namespace default
```

The chart creates and owns the `kaddie-system` namespace, so the Helm release
record lives in another namespace (`default` above). The chart never creates
`SREConfig/default`, so a fresh install is Halted; the install notes print the
command that enables it. Kubernetes 1.28 or later is required.

## License

GPL-3.0; see `LICENSE`.
