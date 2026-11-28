# Spec test traceability

Every acceptance test ID in the specs (base T1–T53, harness HT1–HT38, GitOps
GT1–GT42 without GT38, UI UT1–UT31, Session Store HS1–HS12), the
implementation-plan task(s) that claim it, the release it lands in, and the
tests that implement it. `make traceability` checks this file in CI.

Rules the checker enforces:

- Every spec ID appears exactly once.
- "Implemented by" is `unclaimed`, `dropped (...)`, or a comma-separated list
  of `path:TestName`, optionally starting with `partial:` and ending with a
  parenthesised note of what is left and which task does it.
- Every listed test exists in the named file.
- Every test whose name carries a spec ID (`TestT9_GT6_...`,
  `TestCRD_T8_...`) is listed in that ID's row.
- An ID claimed by a task in "Tasks done" is not `unclaimed`.

Tasks done: T0.1, T0.2, T0.3, T1.1, T1.2, T1.3, T1.4, T1.5, T1.6, T2.1, T2.2, T2.3, T2.4, T2.5, T2.6, T2.7

| ID | Spec | Summary | Planned task(s) | Release | Implemented by |
| --- | --- | --- | --- | --- | --- |
| T1 | base | kaddie-tools and kaddie-agent: can-i on `secrets` (get/list/watch) = no for every namespace. | T1.6 | v1 | test/e2e/rbac_test.go:TestT1_T2_NoSecretsOrDangerousSubresources |
| T2 | base | kaddie-tools and kaddie-agent: no on `pods/exec`, `pods/attach`, `pods/portforward`, `pods/proxy`, `service... | T1.6 | v1 | test/e2e/rbac_test.go:TestT1_T2_NoSecretsOrDangerousSubresources |
| T3 | base | kaddie-agent is bound to no role: its `can-i --list` contains only the Kubernetes-default self-review permi... | T1.6 | v1 | partial: test/e2e/rbac_test.go:TestT3_GT1_AgentAndToolsPermissions (pod token and API server egress parts need the Deployment, T6.9) |
| T4 | base | No SRE service identity (agent, and the harness, git writer, renderer and UI identities) has a forbidden pe... | T1.3, T1.6 | v1 | test/chart/rbac_render_test.go:TestT4_EveryRuleFromATemplate, test/chart/rbac_render_test.go:TestT4_NoWildcardsOrAggregation, test/chart/rbac_render_test.go:TestT4_NoForbiddenPermission, test/e2e/rbac_test.go:TestT4_GT2_GT4_HT1_EffectivePermissionsMatchGolden |
| T5 | base | `kaddie-operator` grants no permission outside the `kaddie.org` group except `get`/`update`/`patch` on the... | T1.6 | v1 | test/chart/rbac_render_test.go:TestT5_OperatorRole, test/e2e/rbac_test.go:TestT5_OperatorRoleAndBindings |
| T6 | base | `kaddie-system` rejects `SREProposal` creates beyond its object quota, and an `SREProposal` in any other na... | T1.2, T2.5 | v1 | test/e2e/chart_test.go:TestT6_ProposalQuota, pkg/proposal/vectors_test.go:TestT6_ProposalNamespaceVectors |
| T7 | base | Creating an `SREProposal` as an identity without `create` on `sreproposals` → denied by RBAC. | T1.4 | v1 | test/e2e/chart_test.go:TestT7_GT5_CreateNeedsRBAC |
| T8 | base | Updating `SREProposal.spec`, even as cluster-admin → denied. | T1.4 | v1 | test/envtest/crd_validation_test.go:TestCRD_T8_SpecImmutable, test/e2e/chart_test.go:TestT8_GT5_SpecImmutableForClusterAdmin |
| T9 | base | A status write by an identity without `update` on `sreproposals/status` → denied by RBAC. | T1.5 | v1 | partial: test/e2e/status_write_test.go:TestT9_GT6_GT40_StatusWrites (D1 firing: T6.11) |
| T10 | base | The API server rejects synchronously: a bad verb including `update`, a missing name, an oversized body, mor... | T1.1 | v1 | test/envtest/crd_test.go:TestCRD_T10_SynchronousRejections |
| T11 | base | A proposal created directly, bypassing kaddie-tools, with an invalid JSON body, a `<redacted:` placeholder... | T2.5, T6.5, T6.7 | v1 | partial: pkg/proposal/vectors_test.go:TestT11_DirectCreationVectors (executor discovery and UI parts: T6.5, T6.7) |
| T12 | base | For each match field in §4.2 (`apiGroups`, `resources` incl. | T2.1, T5.5 | v1 | partial: pkg/proposal/match_test.go:TestT12_RuleFieldsMatch, pkg/proposal/vectors_test.go:TestT12_CatalogueMatchFieldVectors (kaddie-tools and kaddie-ui parts: T5.5, T6.5) |
| T13 | base | A proposal where only one of several requests matches a protection is high risk, and the reason names that... | T2.5 | v1 | pkg/proposal/vectors_test.go:TestT13_FindingNamesTheRequestVectors |
| T14 | base | `generateName` creates and requests on the `namespaces` resource itself can't evade `names` or `namespaces`... | T2.1 | v1 | pkg/proposal/match_test.go:TestT14_GenerateNameAndNamespaceResource, pkg/proposal/vectors_test.go:TestT14_GenerateNameAndNamespaceResourceVectors |
| T15 | base | Every protection in §6.3, including Secrets, secret-delivery kinds, reconciler kinds, all RBAC, the system... | T2.5 | v1 | pkg/proposal/vectors_test.go:TestT15_ProtectionVectors, pkg/proposal/vectors_test.go:TestT15_EveryProtectionHasAMatchingVector |
| T16 | base | The protections and the match semantics are identical in kaddie-ui and kaddie-tools: the shared conformance... | T2.7, T5.5, T6.5, T10.4 | v1 | partial: pkg/proposal/vectors_test.go:TestT16_ConformanceVectors, pkg/proposal/conformance/conformance_test.go:TestT16_CheckReportsMissingAndExtraFindings, pkg/proposal/conformance/conformance_test.go:TestT16_CheckPassesAConformingEngine, pkg/proposal/conformance/conformance_test.go:TestT16_CheckComparesFieldAndSource (consumers: T5.5, T6.5, T10.4) |
| T17 | base | Happy path with a multi-request proposal: the audit log shows each write made by the operator's own identit... | T6.6 | v1 | unclaimed |
| T18 | base | An operator lacking permission for the second of three requests → the first succeeds, the second fails, the... | T6.6 | v1 | unclaimed |
| T19 | base | The client refuses expired proposals, proposals that already have an outcome, and any execution when `Execu... | T6.6, T6.7 | v1 | unclaimed |
| T20 | base | The bytes sent for each request equal the proposal's stored request exactly (no client-side modification). | T6.6 | v1 | unclaimed |
| T21 | base | kaddie-ui never persists or logs credentials (scan logs, metrics, error bodies and browser storage for toke... | T6.3 | v1 | unclaimed |
| T22 | base | The client shows every request body in full, including very large bodies (up to the 64 KiB limit). | T6.6, T6.7 | v1 | unclaimed |
| T23 | base | After a new CRD is installed, discovery reports it as `type-only`, and listing its instances is forbidden. | T5.3 | v1 | unclaimed |
| T24 | base | `propose_read` for that type creates a proposal whose only request is a `patch` on `clusterroles/kaddie-age... | T7.1 | v1.1 | unclaimed |
| T25 | base | `EvaluateStatic` and the exact evaluation flag as high risk any proposal that would leave `kaddie-agent-rea... | T2.5, T2.6 | v1 | pkg/proposal/vectors_test.go:TestT25_AgentReadRoleStaticVectors, pkg/proposal/readrole_test.go:TestT25_ShippedReadRoleRulesPass, pkg/proposal/readrole_test.go:TestT25_ReadRoleRuleViolations, pkg/proposal/vectors_test.go:TestT25_ExactReadRole |
| T26 | base | An `Api` proposal that patches any other RBAC object, creates or deletes any ClusterRole or binding, or pat... | T7.1 | v1.1 | unclaimed |
| T27 | base | In `cluster` ownership mode, `helm upgrade` leaves rules added to `kaddie-agent-read` after install in plac... | T1.3, T7.2 | v1 | partial: test/chart/rbac_render_test.go:TestT27_AgentReadKeptByHelm, test/e2e/chart_test.go:TestT27_AgentReadSurvivesHelm (git mode: T7.2) |
| T28 | base | Removing a rule from `kaddie-agent-read` by hand makes its kind `type-only` again within the discovery refr... | T5.3 | v1 | unclaimed |
| T29 | base | `Halted` → kaddie-tools creates no proposal, the agent makes no LLM calls (mock LLM sees zero requests), an... | T5.2, T5.3, T5.5 | v1 | unclaimed |
| T30 | base | `SREConfig` deleted → everything behaves as `Halted`, including the harness creating no proposal. | T5.5 | v1 | unclaimed |
| T31 | base | L2 and L3 break-glass steps take effect immediately. | T6.11 | v1 | unclaimed |
| T32 | base | Redaction corpus: secrets planted in env values, args, annotations, ConfigMaps, logs, events and granted CR... | T3.2, T5.3, T5.4 | v1 | unclaimed |
| T33 | base | Unsupported built-in kind or redactor failure → payload dropped, nothing sent. | T3.1, T3.2 | v1 | unclaimed |
| T34 | base | A drafted proposal containing a `<redacted:` placeholder is created by kaddie-tools only with a `riskExplan... | T2.5, T5.5 | v1 | partial: pkg/proposal/vectors_test.go:TestT34_RedactedPlaceholderVectors (kaddie-tools part: T5.5) |
| T35 | base | Adversarial-LLM property test: a mock LLM issues random and adversarial tool calls (unknown tools, malforme... | T6.10 | v1 | unclaimed |
| T36 | base | Prompt-injection corpus planted in logs and annotations: the same assertions as T35. | T6.10 | v1 | unclaimed |
| T37 | base | For `create` and each patch type (JSON Patch, JSON merge, strategic merge), a request that changes a protec... | T2.3, T2.4 | v1 | pkg/proposal/vectors_test.go:TestT37_CreateAndEachPatchTypeVectors |
| T38 | base | Ancestors and descendants: replacing `spec.template` wholesale is high risk under the rule on `spec.templat... | T2.2 | v1 | pkg/proposal/fieldpath/match_test.go:TestT38_AncestorsAndDescendants, pkg/proposal/vectors_test.go:TestT38_SubtreeVectors |
| T39 | base | A Deployment patch changing only `spec.replicas` or `resources` isn't flagged; one that also changes `spec.... | T2.3, T2.4 | v1 | pkg/proposal/vectors_test.go:TestT39_ReplicasAndResourcesVectors |
| T40 | base | A test mutating webhook changes a protected field during the dry run → the client flags the proposal as hig... | T2.6, T6.5 | v1 | partial: pkg/proposal/vectors_test.go:TestT40_AdmissionChangesAreFlaggedAndAttributed (kaddie-ui preview and confirmation: T6.5) |
| T41 | base | Field rules never match `delete`. | T2.3 | v1 | pkg/proposal/vectors_test.go:TestT41_DeleteVectors |
| T42 | base | An identity with `update` and `patch` on `sreconfigs` can change `mode`, `triggersPaused` and every setting... | T1.6 | v1 | test/e2e/rbac_test.go:TestT42_GT3_OnlyOperatorsChangeSREConfig |
| T43 | base | Creating an `SREConfig` with any name other than `default` is rejected by the CRD schema. | T1.2, T1.4 | v1 | partial: test/envtest/crd_validation_test.go:TestCRD_T43_SREConfigSingleton, test/chart/render_test.go:TestT43_ChartCreatesNoSREConfig, test/e2e/chart_test.go:TestT43_SREConfigUntouchedByHelm (components behaving as Halted: T4.6, T5.2, T5.3, T6.8) |
| T44 | base | With `api.enabled: false`, kaddie-tools refuses to create an `Api` proposal, except one whose single reques... | T5.5 | v1 | unclaimed |
| T45 | base | With proposals.expirySeconds unset, no proposal expires; with it set, a proposal expires at its creation ti... | T2.6 | v1 | pkg/proposal/canact_test.go:TestT45_CanActExpiry, pkg/proposal/canact_test.go:TestT45_ExpiryIgnoresTheSpec |
| T46 | base | kaddie-sessions' retention loop never removes an open proposal, high risk or not, and removes closed ones (... | T4.8 | v1 | unclaimed |
| T47 | base | A read proposal an operator dismissed is not proposed again for the same type in the same session, and `pro... | T7.1 | v1.1 | unclaimed |
| T48 | base | In `git` ownership mode, `propose_read` edits only the configured file, kaddie-tools refuses to build an `A... | T7.2, T12.10 | v1.1 | unclaimed |
| T49 | base | A proposal whose `sessionId`/`runId` reference a deleted or tampered session is evaluated and executable ex... | T6.10, T8.6 | v1 | unclaimed |
| T50 | base | No pod can connect to the kaddie-agent except the metrics scrape, the agent's egress is limited to the LLM... | T6.9, T11.1 | v1 | unclaimed |
| T51 | base | A call to an unlisted external tool, a drifted MCP definition or an invalid parameter is rejected with no o... | T9.3 | v1.1 | unclaimed |
| T52 | base | Secrets planted in human messages, webhook payloads and external tool results reach the LLM and the session... | T9.1 | v1.1 | unclaimed |
| T53 | base | Rendering the chart for a cluster reporting Kubernetes 1.27 fails with the message naming the requirement a... | T1.2 | v1 | test/chart/render_test.go:TestT53_VersionGate, test/e2e/chart_test.go:TestT53_InstallsOnMinimumVersion |
| HT1 | harness | kaddie-agent's and kaddie-tools' effective permissions are exactly base T3's sets: the harness adds nothing. | T1.6 | v1 | test/e2e/rbac_test.go:TestHT1_SessionsReadsToolsPlusTriggers, test/e2e/rbac_test.go:TestT4_GT2_GT4_HT1_EffectivePermissionsMatchGolden, test/e2e/rbac_test.go:TestT3_GT1_AgentAndToolsPermissions |
| HT2 | harness | No pod can open a connection to the kaddie-agent pod on any port except the metrics port from the scrape id... | T6.9, T11.1 | v1 | unclaimed |
| HT3 | harness | A kaddie-agent token with the default audience, or with audience `kaddie-tools`, is rejected by kaddie-sess... | T4.4 | v1 | unclaimed |
| HT4 | harness | The kaddie-agent pod spec contains no Secret or env from any external tool or store credential; a planted c... | T6.9, T9.1, T11.1 | v1 | unclaimed |
| HT5 | harness | Conformance suite passes over gRPC for every first-party driver sidecar and for a reference custom driver (... | T4.2, T11.2, T11.3 | v1 | unclaimed |
| HT6 | harness | Two kaddie-sessions instances racing (leader election disabled in a test build): at most one append succeed... | T4.2, T4.5 | v1 | unclaimed |
| HT7 | harness | Modifying a stored record in the backend makes the session `IntegrityFailed` on next read, and changes noth... | T4.7 | v1 | unclaimed |
| HT8 | harness | A proposal whose `sessionId`/`runId` reference a deleted or tampered session is evaluated and executable ex... | T6.10 | v1 | unclaimed |
| HT9 | harness | Secrets planted in a human message, a webhook payload, a run summary and an external tool result reach the... | T4.6, T8.4, T9.1 | v1 | unclaimed |
| HT10 | harness | Proposals per session: a session with 50 completed runs still accepts a message and queues a run, and each... | T4.8, T5.5 | v1 | unclaimed |
| HT11 | harness | With `retentionDays` unset, no session is ever deleted automatically; with it set, a session is deleted `re... | T4.8, T11.4 | v1 | unclaimed |
| HT12 | harness | A user without `create sresessions` can't start a session; one without `get` can't read; a user without `de... | T4.4, T4.6 | v1 | unclaimed |
| HT13 | harness | A session that read logs in namespaces `a` and `b` is readable by a user with `get pods/log` in both, invis... | T4.4, T4.7, T9.2 | v1 | unclaimed |
| HT14 | harness | kaddie-sessions rejects an `Kaddie-Assertion` header on a request authenticated as any identity other than... | T4.4 | v1 | unclaimed |
| HT15 | harness | Identity: with username and group prefixes configured on the kube-apiserver, the subject kaddie-sessions pa... | T0.3, T4.4 | v1 | partial: test/e2e/oidc_spike_test.go:TestOIDC_Spike (prefixes applied by the API server; assertions: T4.4) |
| HT38 | harness | `ListSessions` filters by `s``ource`, `trigger`, `namespace`, `object`, `proposal` and time window return e... | T4.7, T10.1 | v1 | unclaimed |
| HT16 | harness | Each shipped `watch` trigger, once promoted to `Active` (with its kinds added to `kaddie-agent-read` where... | T8.2, T8.3 | v1.1 | unclaimed |
| HT17 | harness | `Shadow` mode starts nothing and records each fire on Shadow-mode instances (`countFired`, `lastFired`); sw... | T8.2 | v1.1 | unclaimed |
| HT18 | harness | `proposal-outcome` resumes the owning session when a proposal is `Executed`, `Dismissed` (for `Git`, by an... | T8.2 | v1.1 | unclaimed |
| HT19 | harness | A webhook without a valid signature or bearer token is rejected before the body is parsed; a 65 KiB body is... | T8.4 | v1.1 | unclaimed |
| HT20 | harness | A `watch` on a resource the agent can't read makes the trigger `Valid: False`; after the kind is added to `... | T8.2, T8.4 | v1.1 | unclaimed |
| HT21 | harness | Trigger-independence property: the same mock-LLM tool calls, issued in a human-started and a trigger-starte... | T8.6 | v1.1 | unclaimed |
| HT22 | harness | A call to an unlisted MCP tool, a listed tool whose definition hash changed, an `http` parameter failing it... | T9.2, T9.3, T11.9 | v1.1 | unclaimed |
| HT23 | harness | No request leaves kaddie-tools to any host other than the configured sources; a tool definition or result c... | T9.3, T11.1 | v1.1 | unclaimed |
| HT24 | harness | Namespace attribution: a PromQL query with `namespace="a"` on every selector attributes `a`; one without ma... | T9.2 | v1.1 | unclaimed |
| HT25 | harness | In `Halted`: kaddie-sessions refuses starts and claims, keeps serving reads and fires no trigger; kaddie-to... | T4.6, T8.2, T9.1 | v1 | unclaimed |
| HT27 | harness | For a run that triggered clearing and summarization, the `context_assembled` and `compaction` records exist... | T5.6 | v1 | unclaimed |
| HT28 | harness | `update_case_file` rejects an extra field, an oversized text, an unknown proposal name and an invisible cha... | T5.6 | v1 | unclaimed |
| HT29 | harness | Clearing never touches the `keepRecentToolResults` most recent results, never splits a call from its result... | T5.6 | v1 | unclaimed |
| HT30 | harness | A mock provider that compacts opaquely (no summary text returned) is refused by the LLM interface, and the... | T5.1 | v1 | unclaimed |
| HT31 | harness | An instruction planted in run 1's tool results reaches run 2 only inside a data block or a typed case-file... | T5.6, T6.10 | v1 | unclaimed |
| HT35 | harness | Every `context_assembled` record carries `promptVersion` and a hash equal to the hash of the shipped prompt... | T5.2 | v1 | unclaimed |
| HT36 | harness | The kaddie-agent's only LLM egress is the configured endpoint (network capture); changing `llm.endpoint` wi... | T6.9 | v1 | unclaimed |
| HT37 | harness | With the prompt set replaced by an adversarial one in a test build (instructions inverted), base T35/T36 an... | T6.10, T11.5 | v1 | unclaimed |
| HT32 | harness | Killing the agent mid-run, once after a read tool call and before its result, and once after a `propose` th... | T5.7 | v1 | unclaimed |
| HT33 | harness | Fencing: an agent whose lease expired during a simulated partition gets `StaleLease` on `ReadRun`, `Append`... | T4.3, T4.5 | v1 | unclaimed |
| HT34 | harness | A run whose agent crashes on every attempt ends `Failed/ResumeLimit` after `maxAttempts`; the time between... | T4.3, T5.7 | v1 | unclaimed |
| HT26 | harness | A mock LLM issues random and adversarial tool calls including external tools, and a mock tenant floods trig... | T11.5 | v1.2 | unclaimed |
| GT1 | gitops | kaddie-agent's and kaddie-tools' effective permissions are exactly base T3's sets, and nothing else. | T12.10 | v2 | partial: test/e2e/rbac_test.go:TestT3_GT1_AgentAndToolsPermissions (re-run with GitOps on: T12.10) |
| GT2 | gitops | kaddie-git-writer's and kaddie-renderer's effective permissions are exactly §6.2's and §6.3's. | T12.10 | v2 | partial: test/e2e/rbac_test.go:TestT4_GT2_GT4_HT1_EffectivePermissionsMatchGolden (re-run with workloads: T12.10) |
| GT3 | gitops | No SRE identity has write access to `sreconfigs`. | T12.10 | v2 | test/e2e/rbac_test.go:TestT42_GT3_OnlyOperatorsChangeSREConfig |
| GT4 | gitops | The base spec's forbidden-permissions test (base T4) passes for every identity in this spec, with no except... | T12.10 | v2 | test/e2e/rbac_test.go:TestT4_GT2_GT4_HT1_EffectivePermissionsMatchGolden |
| GT5 | gitops | Base T7 and T8 hold for `Git` proposals. | T12.10 | v2 | test/e2e/chart_test.go:TestT7_GT5_CreateNeedsRBAC, test/e2e/chart_test.go:TestT8_GT5_SpecImmutableForClusterAdmin |
| GT6 | gitops | In normal operation each `Git` status block is written only by its intended writer (§7.2): `render` by kadd... | T1.5, T12.5, T12.10 | v1 | partial: test/e2e/status_write_test.go:TestT9_GT6_GT40_StatusWrites (GD1 firing: T12.10) |
| GT7 | gitops | The API server rejects synchronously: more than 20 edits, an unknown operation, missing or extra fields for... | T1.1, T12.2 | v1 | partial: test/envtest/crd_test.go:TestCRD_GT7_GitSchemaRejections (kaddie-tools and UI part: T12.2) |
| GT8 | gitops | kaddie-git-writer scaled to 0: proposals are stored, rendered and can be approved, no PR is opened, and not... | T12.7 | v2 | unclaimed |
| GT9 | gitops | A created plain manifest declaring a ClusterRoleBinding, a Secret, an ExternalSecret, an Argo CD Applicatio... | T12.2, T12.5 | v2 | unclaimed |
| GT10 | gitops | Proposals touching CI definitions, CODEOWNERS, `.gitattributes`, `.gitmodules`, symlinks, `Chart.yaml` depe... | T12.7 | v2 | unclaimed |
| GT11 | gitops | Edits to files that no deploy path consumes render as `Failed` (`UnconsumedPath`), and kaddie-ui flags them... | T12.3 | v2 | unclaimed |
| GT12 | gitops | A deploy path's `extraArgs` reach `helm template` or `kubectl kustomize` as separate arguments, unchanged a... | T12.3 | v2 | unclaimed |
| GT13 | gitops | A `baseSHA` that isn't an ancestor of the tip, or is more than 500 commits old, fails. | T12.2, T12.7 | v2 | unclaimed |
| GT14 | gitops | kaddie-tools refuses to read a path in `protectedPaths` and refuses a proposal that creates, edits or delet... | T12.3 | v2 | unclaimed |
| GT15 | gitops | A YAML alias bomb or a deeply nested document in a created file hits the parser limits; the proposal is hig... | T12.2 | v2 | unclaimed |
| GT35 | gitops | For fixture repositories (Plain, Helm with values files, Kustomize overlays), `status.render.requests` equa... | T12.4 | v2 | unclaimed |
| GT36 | gitops | The `render-sandbox` container has no ServiceAccount token, no git credential and no writable root filesyst... | T12.4 | v2 | unclaimed |
| GT37 | gitops | A deploy path whose render fails (for example a missing values file) is `Unrenderable`; kaddie-ui shows a w... | T12.4 | v2 | unclaimed |
| GT39 | gitops | kaddie-ui shows a Git proposal's rendered requests through the same view, static evaluation and dry-run as... | T12.5 | v2 | unclaimed |
| GT40 | gitops | kaddie-ui offers `ReadyForPR` and `Dismissed` only to a viewer whose SelfSubjectAccessReview for `update` o... | T1.5, T12.5 | v1 | partial: test/e2e/status_write_test.go:TestT9_GT6_GT40_StatusWrites (kaddie-ui part: T12.5) |
| GT41 | gitops | kaddie-ui offers `ReadyForPR` only to viewers allowed to update `sreproposals/status`, disables it when the... | T12.5 | v2 | unclaimed |
| GT16 | gitops | kaddie-git-writer ignores proposals without `ReadyForPR`, `Dismissed` ones, expired ones, other types, ones... | T12.7 | v2 | unclaimed |
| GT17 | gitops | kaddie-git-writer pushes exactly one commit to a new `kaddie/<uid>` branch, with `baseSHA` as its parent, n... | T12.7, T12.8 | v2 | unclaimed |
| GT42 | gitops | An approval whose `headTreeSHA` differs from the tree kaddie-git-writer computes (simulated edit-library sk... | T12.7 | v2 | unclaimed |
| GT18 | gitops | The PR title and body contain no unescaped LLM text. | T12.8 | v2 | unclaimed |
| GT19 | gitops | Preflight: each missing setting in §9.1 blocks PR creation for that repository; attestations are reflected... | T12.9 | v2 | unclaimed |
| GT20 | gitops | A foreign commit on a PR branch causes failure status and closure. | T12.9 | v2 | unclaimed |
| GT21 | gitops | kaddie-git-writer's credential can't merge, approve or push to the target branch (verified against the test... | T12.6 | v2 | unclaimed |
| GT22 | gitops | Adding a `protectedPaths` entry that covers an edited file between the check and the push makes kaddie-git-... | T12.3 | v2 | unclaimed |
| GT23 | gitops | Adding a `protectedPaths` entry that matches an open PR's file makes kaddie-git-writer comment and update t... | T12.9 | v2 | unclaimed |
| GT24 | gitops | With an expiry configured, expiry closes the PR or ends an unapproved proposal; with none, neither happens. | T12.9 | v2 | unclaimed |
| GT25 | gitops | A proposal closed by a reviewer, dismissed by an operator, or not pushable by kaddie-git-writer, is removed... | T12.9 | v2 | unclaimed |
| GT26 | gitops | `ExecutionPaused`: proposals are still rendered, `ReadyForPR` is disabled in kaddie-ui, no new PRs are open... | T12.9 | v2 | unclaimed |
| GT27 | gitops | `Halted`, or a deleted `SREConfig`: the agent makes no LLM calls, kaddie-renderer renders nothing, and kadd... | T12.9 | v2 | unclaimed |
| GT28 | gitops | GL2, GL3, GL4 and GL4a take effect immediately. | T12.9 | v2 | unclaimed |
| GT29 | gitops | Planted secrets in repository files reach the LLM 0 times (mock LLM records everything). | T12.1 | v2 | unclaimed |
| GT30 | gitops | Repository tools can't read history, other refs, unregistered repositories, or paths in `protectedPaths`; n... | T12.1 | v2 | unclaimed |
| GT31 | gitops | Adversarial-LLM property test (base T35, extended): random and adversarial tool calls and git proposals. | T12.10 | v2 | unclaimed |
| GT32 | gitops | An `Api` proposal, a declared manifest or a rendered request that targets a reconciler kind (Argo CD or Flu... | T12.3 | v2 | unclaimed |
| GT33 | gitops | With `api.enabled: false`, `Git` proposals and read proposals work: a `propose_read` for a CRD can be execu... | T12.10 | v2 | unclaimed |
| GT34 | gitops | Changing one repository entry re-checks only that repository's open PRs. | T12.9 | v2 | unclaimed |
| UT1 | ui | The kaddie-ui pod mounts no ServiceAccount token except the projected one with audience `kaddie-sessions`,... | T6.1 | v1 | unclaimed |
| UT2 | ui | Sign-in yields one kube token. | T6.2 | v1 | unclaimed |
| UT3 | ui | With username and group prefixes configured, the identity kaddie-ui displays, the `actedBy` it writes and t... | T0.3, T6.2 | v1 | partial: test/e2e/oidc_spike_test.go:TestOIDC_Spike (prefixes applied by the API server; kaddie-ui part: T6.2) |
| UT4 | ui | Tokens never appear in kaddie-ui logs, metrics, error bodies or in `localStorage`, cookies or IndexedDB; `s... | T6.1, T6.3 | v1 | unclaimed |
| UT5 | ui | Sessions routes reach kaddie-sessions with kaddie-ui's token and an assertion carrying the username, UID, g... | T6.2 | v1 | unclaimed |
| UT6 | ui | Happy path with a multi-request `Api` proposal: the audit log shows each write by the operator's identity w... | T6.6 | v1 | unclaimed |
| UT7 | ui | Lacking permission for the second of three requests: first succeeds, second fails, third `Skipped`, status... | T6.6 | v1 | unclaimed |
| UT8 | ui | Execute is refused, with the reason shown, for: expired, already has an outcome, `ExecutionPaused`, `Halted... | T6.6 | v1 | unclaimed |
| UT9 | ui | The bytes sent for each request equal the stored request exactly, including a 64 KiB body (T20, T22). | T6.6, T6.7 | v1 | unclaimed |
| UT10 | ui | Preview-to-execute integrity: opening a proposal's review view from the inbox or the list starts its previe... | T6.6 | v1 | unclaimed |
| UT11 | ui | A test mutating webhook changing a protected field during the dry run: the preview attributes the change to... | T6.5 | v1 | unclaimed |
| UT12 | ui | A read proposal in `cluster` ownership mode: the one-rule patch and the resulting role are shown in full; e... | T7.1, T7.3 | v1.1 | unclaimed |
| UT13 | ui | The simulator's static evaluation, the proposal list's high-risk flag and kaddie-tools' pre-check agree for... | T10.4 | v1.2 | unclaimed |
| UT14 | ui | A hostile transcript corpus (markdown, HTML, `<script>`, `<img>`, links, data URLs, bidi and zero-width cha... | T6.3, T6.4 | v1 | unclaimed |
| UT15 | ui | While a proposal exists, its card shows the live state and kaddie-ui's own evaluation and ignores a transcr... | T6.4 | v1 | unclaimed |
| UT16 | ui | Provenance, namespaces touched, cluster-wide flag, budget counters, participants, run reason codes, resume... | T6.4 | v1 | unclaimed |
| UT17 | ui | Live follow: records appear as the mock agent appends them; when the agent reads a namespace the viewer can... | T6.4 | v1 | unclaimed |
| UT18 | ui | Posting a message containing a planted credential shows the redaction rejection and stores nothing; a 17 Ki... | T6.8 | v1 | unclaimed |
| UT19 | ui | `Halted`: history loads, the composer and start controls are disabled with the reason, Execute is absent; `... | T6.8 | v1 | unclaimed |
| UT20 | ui | Inbox: a trigger-started session's finished run, an actionable proposal and a high-risk proposal each appea... | T10.3 | v1.2 | unclaimed |
| UT21 | ui | Trigger workbench: a trigger's instances list each subject with `currentlyFiring`, `countFired` and `lastFi... | T8.5 | v1.1 | unclaimed |
| UT22 | ui | Stateless deployment: with two kaddie-ui replicas behind a load balancer, and then with kaddie-ui running o... | T11.7 | v1.2 | unclaimed |
| UT23 | ui | User state: saved filters and pins written in one browser appear in another after sign-in; a planted creden... | T10.1 | v1.2 | unclaimed |
| UT24 | ui | Scale: a 10 000-record, 64 MiB session opens, scrolls and follows; a namespace with 5 000 pods lists with p... | T11.7 | v1.2 | unclaimed |
| UT25 | ui | RBAC degradation: a viewer without `get` on the ClusterRole `kaddie-agent-read` sees the read-scope table w... | T7.3, T10.2 | v1.1 | unclaimed |
| UT26 | ui | Deep links: every route in §7 opens for an authorized viewer and shows the §5.5 message for an unauthorized... | T6.3, T10.2, T11.7 | v1 | unclaimed |
| UT27 | ui | Proxy allowlist: a crafted request through `/api/k8s` to `pods/exec`, to a `deletecollection`, to an `imper... | T6.2, T11.7 | v1 | unclaimed |
| UT28 | ui | Only `status.pr.url` whose host matches the registered repository is rendered as a link; one pointing elsew... | T11.7 | v1.2 | unclaimed |
| UT29 | ui | A `Git` proposal's rendered requests are shown, statically evaluated and dry-run through the same component... | T12.5 | v2 | unclaimed |
| UT30 | ui | `ReadyForPR` and Dismiss are offered only to a viewer allowed to update `sreproposals/status`, disabled out... | T12.5 | v2 | unclaimed |
| UT31 | ui | Startup self-check (§4.5): with an unreachable issuer, a missing client ID, or a missing `kaddie-sessions`-... | T6.1 | v1 | unclaimed |
| HS1 | session store | Every operation sent to a standby returns the same status and body as one sent to the leader. | T11.8 | v1.2 | unclaimed |
| HS2 | session store | Under steady load (claims, appends, follows, posts), a rolling update with replicas 1 and with replicas 2 l... | T11.8 | v1.2 | unclaimed |
| HS3 | session store | A second instance forced to act as leader (election disabled) gets ErrConflict on its first append, exits,... | T4.5 | v1 | unclaimed |
| HS4 | session store | Killing the leader after a mid-run message is stored queues exactly one follow-up once the run ends. | T4.5, T11.8 | v1 | unclaimed |
| HS5 | session store | Every §5.2 transition happens as tabled, every other one is refused with InvalidState, and no run is ever E... | T4.3 | v1 | unclaimed |
| HS6 | session store | A waiting ClaimRun returns within 1 s of a run becoming eligible and returns 204 at its wait limit. | T4.3 | v1 | unclaimed |
| HS7 | session store | A lease that expires while Halted ends the run Aborted, Halted, and never requeues it | T4.3 | v1 | unclaimed |
| HS8 | session store | An Append retried with the same batchSeq after a dropped response is stored once. | T4.5, T4.6 | v1 | unclaimed |
| HS9 | session store | Export and import need delete on sresessions; export applies visibility; import verifies the chain, refuses... | T11.4 | v1.2 | unclaimed |
| HS10 | session store | A webhook trigger naming an unmounted Secret is Valid: False, SecretNotMounted, and no request to its path... | T8.4 | v1.1 | unclaimed |
| HS11 | session store | Trigger instances survive a takeover: after a leader change or restart, a firing instance whose condition s... | T8.2 | v1.1 | unclaimed |
| HS12 | session store | The chart renders no PDB at replicas 1 and minAvailable 1 at 2; refuses replicas > 1 with driver filesystem... | T4.5, T11.8 | v1 | unclaimed |
