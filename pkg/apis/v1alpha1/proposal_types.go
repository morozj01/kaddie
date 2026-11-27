package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProposalType says which write path a proposal uses.
// +kubebuilder:validation:Enum=Api;Git
type ProposalType string

const (
	// ProposalTypeAPI is a list of Kubernetes API requests a human executes.
	ProposalTypeAPI ProposalType = "Api"
	// ProposalTypeGit is a set of file edits to a registered repository.
	ProposalTypeGit ProposalType = "Git"
)

// SREProposal is an untrusted suggestion drafted by the agent for a human to
// review. It authorizes nothing by itself. Its spec is immutable after create.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name=Type,type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name=Phase,type=string,JSONPath=`.status.execution.phase`
// +kubebuilder:printcolumn:name=Render,type=string,JSONPath=`.status.render.phase`
// +kubebuilder:printcolumn:name=Approval,type=string,JSONPath=`.status.approval.phase`
// +kubebuilder:printcolumn:name=PR,type=string,JSONPath=`.status.pr.state`
// +kubebuilder:printcolumn:name=Age,type=date,JSONPath=`.metadata.creationTimestamp`
type SREProposal struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the suggestion itself, immutable after create.
	Spec SREProposalSpec `json:"spec"`
	// Status records what happened; written through the status endpoint only.
	// +optional
	Status SREProposalStatus `json:"status,omitempty"`
}

// SREProposalSpec is the content of a proposal.
//
// Base §4.1's 256 KiB limit on the whole spec is not a CRD rule: summing the
// text fields needs CEL's sum(), whose estimated cost a 1.28 API server
// rejects as more than 100x over budget. Base §6.1 doesn't list it among the
// synchronous checks; it belongs to the static checks in pkg/proposal.
//
// +kubebuilder:validation:XValidation:rule="self.type == 'Api' ? (has(self.api) && !has(self.git)) : (has(self.git) && !has(self.api))",message="exactly the block matching spec.type must be set"
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable"
type SREProposalSpec struct {
	// Type says which write path the proposal uses.
	Type ProposalType `json:"type"`
	// API holds direct API requests, for type Api.
	// +optional
	API *APIProposal `json:"api,omitempty"`
	// Git holds file edits, for type Git.
	// +optional
	Git *GitProposal `json:"git,omitempty"`
	// Justification is LLM-written, untrusted plain text.
	// +kubebuilder:validation:MaxLength=4000
	Justification string `json:"justification"`
	// RiskExplanation is LLM-written and untrusted: why each flagged change
	// can't be avoided. kaddie-tools requires it when a risk check flags the
	// proposal. Risk itself is never stored.
	// +kubebuilder:validation:MaxLength=4000
	// +optional
	RiskExplanation string `json:"riskExplanation,omitempty"`
	// Evidence lists object references the agent cites; informational only.
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Evidence []EvidenceRef `json:"evidence,omitempty"`
	// SessionID names the session that drafted the proposal, for correlation only.
	// +kubebuilder:validation:MaxLength=64
	// +optional
	SessionID string `json:"sessionId,omitempty"`
	// RunID names the run that drafted the proposal, for correlation only.
	// +kubebuilder:validation:MaxLength=64
	// +optional
	RunID string `json:"runId,omitempty"`
}

// APIProposal is the Api block: requests executed in order.
type APIProposal struct {
	// Requests are executed in order; the first failure stops the rest.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=20
	Requests []APIRequest `json:"requests"`
}

// RequestVerb is the verb of a proposed request. update, deletecollection and
// CONNECT verbs are deliberately impossible to express.
// +kubebuilder:validation:Enum=create;patch;delete
type RequestVerb string

// PatchType is one of the three patch formats the protections can analyse.
// +kubebuilder:validation:Enum=application/json-patch+json;application/merge-patch+json;application/strategic-merge-patch+json
type PatchType string

// RequestFields are the fields of one Kubernetes API request. They are shared
// by spec.api.requests and status.render.requests so both have one shape.
type RequestFields struct {
	// Verb is create, patch or delete.
	Verb RequestVerb `json:"verb"`
	// APIVersion is the group/version, or v1 for the core group.
	// +kubebuilder:validation:MaxLength=253
	APIVersion string `json:"apiVersion"`
	// Resource is the plural resource name.
	// +kubebuilder:validation:MaxLength=253
	Resource string `json:"resource"`
	// Subresource is an optional subresource such as scale.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	Subresource string `json:"subresource,omitempty"`
	// Namespace is empty for cluster-scoped resources.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// Name is the target object name.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Name string `json:"name,omitempty"`
	// PatchType is required for patch.
	// +optional
	PatchType PatchType `json:"patchType,omitempty"`
	// Body is the exact request bytes as a JSON string, at most 64 KiB.
	// +kubebuilder:validation:MaxLength=65536
	// +optional
	Body string `json:"body,omitempty"`
}

// Markers whose CEL contains '' live in a comment group of their own, apart
// from the doc comment: gofmt rewrites '' to a typographic quote in doc
// comments, and controller-gen reads this group too.
//
// +kubebuilder:validation:XValidation:rule="self.verb == 'create' || (has(self.name) && self.name != '')",message="name is required for patch and delete"
// +kubebuilder:validation:XValidation:rule="self.verb != 'patch' || (has(self.patchType) && self.patchType != '')",message="patchType is required for patch"
// +kubebuilder:validation:XValidation:rule="self.verb == 'delete' || (has(self.body) && self.body != '')",message="body is required for create and patch"

// APIRequest is one request of an Api proposal.
type APIRequest struct {
	RequestFields `json:",inline"`
}

// RenderedRequest is one request kaddie-renderer recorded: exactly the item
// shape of spec.api.requests. Advisory; nothing ever executes it.
type RenderedRequest struct {
	RequestFields `json:",inline"`
}

// GitProposal is the Git block: file edits to one registered repository.
type GitProposal struct {
	// Repository names a registered repository entry in SREConfig.
	// +kubebuilder:validation:MaxLength=63
	Repository string `json:"repository"`
	// BaseSHA is a full SHA-1 or SHA-256 commit ID.
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{40}([0-9a-f]{24})?$`
	BaseSHA string `json:"baseSHA"`
	// Edits are applied in order.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=20
	// +kubebuilder:validation:XValidation:rule="self.all(e, self.filter(f, f.path == e.path).size() == 1)",message="no two edits may target the same path"
	Edits []GitEdit `json:"edits"`
}

// EditOperation says what an edit does to its file.
// +kubebuilder:validation:Enum=Create;Edit;Delete
type EditOperation string

// Kept apart from the doc comment for the same reason as APIRequest's.
//
// +kubebuilder:validation:XValidation:rule="self.operation != 'Edit' || (has(self.oldText) && has(self.newText))",message="Edit requires oldText and newText"
// +kubebuilder:validation:XValidation:rule="self.operation != 'Create' || has(self.content)",message="Create requires content"
// +kubebuilder:validation:XValidation:rule="self.operation == 'Edit' ? !has(self.content) : (self.operation == 'Create' ? (!has(self.oldText) && !has(self.newText)) : (!has(self.content) && !has(self.oldText) && !has(self.newText)))",message="an edit carries only the fields its operation uses"
// +kubebuilder:validation:XValidation:rule="!self.path.startsWith('/') && !self.path.contains('\\\\') && !self.path.matches('[[:cntrl:]]') && !self.path.split('/').exists(s, s in ['', '.', '..'])",message="path must be relative and /-separated, with no empty, . or .. segments, backslashes or control characters"

// GitEdit is one file edit.
type GitEdit struct {
	// Path is repository-relative; canonical form is checked by pkg/gitproposal.
	// +kubebuilder:validation:MaxLength=1024
	Path string `json:"path"`
	// Operation is Create, Edit or Delete.
	Operation EditOperation `json:"operation"`
	// OldText must occur exactly once in the file (Edit).
	// +kubebuilder:validation:MaxLength=65536
	// +optional
	OldText *string `json:"oldText,omitempty"`
	// NewText replaces OldText (Edit).
	// +kubebuilder:validation:MaxLength=65536
	// +optional
	NewText *string `json:"newText,omitempty"`
	// Content is the full file content (Create).
	// +kubebuilder:validation:MaxLength=65536
	// +optional
	Content *string `json:"content,omitempty"`
}

// EvidenceRef is an object reference the agent cites. Never used for decisions.
type EvidenceRef struct {
	// APIGroup is empty for the core group.
	// +optional
	APIGroup string `json:"apiGroup,omitempty"`
	// Kind is the object kind.
	Kind string `json:"kind"`
	// Namespace is empty for cluster-scoped objects.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// Name is the object name.
	Name string `json:"name"`
}

// SREProposalStatus records what happened. Each block has one intended
// writer; RBAC can't enforce which. Status is a record, not an authorization.
type SREProposalStatus struct {
	// Execution is the Api outcome, written by kaddie-ui as the acting human.
	// +optional
	Execution *ExecutionStatus `json:"execution,omitempty"`
	// Render is what kaddie-renderer rendered; advisory, never executed.
	// +optional
	Render *RenderStatus `json:"render,omitempty"`
	// Approval is an operator's decision on a Git proposal, written through
	// kaddie-ui as that operator.
	// +optional
	Approval *ApprovalStatus `json:"approval,omitempty"`
	// PR is the Git PR state, written by kaddie-git-writer.
	// +optional
	PR *PRStatus `json:"pr,omitempty"`
	// PushFailure says why kaddie-git-writer couldn't push an approved proposal.
	// +optional
	PushFailure *PushFailure `json:"pushFailure,omitempty"`
}

// ExecutionPhase is the outcome of an Api proposal; absent while open.
// +kubebuilder:validation:Enum=Executed;PartiallyExecuted;Failed;Dismissed
type ExecutionPhase string

// ExecutionStatus is the Api outcome. kaddie-ui never rewrites it once set.
type ExecutionStatus struct {
	// Phase is the outcome.
	// +optional
	Phase ExecutionPhase `json:"phase,omitempty"`
	// ActedBy is the acting human, set by kaddie-ui; the audit log records
	// the real author.
	// +optional
	ActedBy string `json:"actedBy,omitempty"`
	// ActedAt is when the human acted.
	// +optional
	ActedAt *metav1.Time `json:"actedAt,omitempty"`
	// Results has one entry per request.
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Results []RequestResult `json:"results,omitempty"`
}

// RequestOutcome is what happened to one request.
// +kubebuilder:validation:Enum=Succeeded;Failed;Skipped
type RequestOutcome string

// RequestResult is the result of one request.
type RequestResult struct {
	// Index is the request position.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Index *int64 `json:"index,omitempty"`
	// Outcome is what happened to that request.
	// +optional
	Outcome RequestOutcome `json:"outcome,omitempty"`
	// Message is the API server's message, truncated.
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Message string `json:"message,omitempty"`
}

// RenderPhase is the outcome of a render.
// +kubebuilder:validation:Enum=Rendered;Unrenderable;Failed
type RenderPhase string

// RenderTool says how a deploy path is rendered.
// +kubebuilder:validation:Enum=Plain;Helm;Kustomize
type RenderTool string

// RenderStatus is what kaddie-renderer recorded for a Git proposal.
type RenderStatus struct {
	// Phase is the outcome of the render.
	// +optional
	Phase RenderPhase `json:"phase,omitempty"`
	// Reason is a stable code such as UnconsumedPath, TooLarge or SandboxLimit.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	Reason string `json:"reason,omitempty"`
	// HeadTreeSHA is the tree of baseSHA with the edits applied.
	// +kubebuilder:validation:MaxLength=64
	// +optional
	HeadTreeSHA string `json:"headTreeSHA,omitempty"`
	// RendererVersion is the renderer and pinned tool versions.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	RendererVersion string `json:"rendererVersion,omitempty"`
	// RenderedAt is when it rendered.
	// +optional
	RenderedAt *metav1.Time `json:"renderedAt,omitempty"`
	// Targets are the affected deploy paths.
	// +kubebuilder:validation:MaxItems=50
	// +optional
	Targets []RenderTarget `json:"targets,omitempty"`
	// Requests are the rendered requests; a larger render is recorded as
	// Failed (TooLarge).
	// +kubebuilder:validation:MaxItems=200
	// +optional
	Requests []RenderedRequest `json:"requests,omitempty"`
}

// RenderTarget is one affected deploy path.
type RenderTarget struct {
	// +optional
	Name string `json:"name,omitempty"`
	// +optional
	Tool RenderTool `json:"tool,omitempty"`
	// Namespace is the namespace for rendered objects that have none.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// Rendered is false when the deploy path is Unrenderable.
	// +optional
	Rendered *bool `json:"rendered,omitempty"`
	// Reason says why it couldn't be rendered.
	// +optional
	Reason string `json:"reason,omitempty"`
	// RequestIndexes lists which requests came from this deploy path.
	// +kubebuilder:validation:items:Minimum=0
	// +optional
	RequestIndexes []int64 `json:"requestIndexes,omitempty"`
}

// ApprovalPhase is an operator's decision on a Git proposal.
// +kubebuilder:validation:Enum=ReadyForPR;Dismissed
type ApprovalPhase string

// ApprovalStatus is an operator's decision. kaddie-ui never rewrites it once set.
type ApprovalStatus struct {
	// Phase is ReadyForPR or Dismissed.
	Phase ApprovalPhase `json:"phase"`
	// ActedBy is the operator, set by kaddie-ui; the audit log records the
	// real author.
	ActedBy string `json:"actedBy"`
	// ActedAt is when the operator decided.
	// +optional
	ActedAt *metav1.Time `json:"actedAt,omitempty"`
	// HeadTreeSHA is the render's tree the operator saw; kaddie-git-writer
	// pushes only this tree.
	// +kubebuilder:validation:MaxLength=64
	// +optional
	HeadTreeSHA string `json:"headTreeSHA,omitempty"`
}

// PRState is the state of a pull request.
// +kubebuilder:validation:Enum=Open;Merged;Closed
type PRState string

// PRStatus is the pull request kaddie-git-writer opened.
type PRStatus struct {
	// +optional
	URL string `json:"url,omitempty"`
	// +optional
	Number *int64 `json:"number,omitempty"`
	// Branch is kaddie/<proposalUID>.
	// +optional
	Branch string `json:"branch,omitempty"`
	// HeadSHA is the pushed commit.
	// +optional
	HeadSHA string `json:"headSHA,omitempty"`
	// BaseSHA is the parent of the pushed commit: spec.git.baseSHA.
	// +optional
	BaseSHA string `json:"baseSHA,omitempty"`
	// HeadTreeSHA is the tree of the pushed commit; equals
	// status.approval.headTreeSHA.
	// +optional
	HeadTreeSHA string `json:"headTreeSHA,omitempty"`
	// +optional
	State PRState `json:"state,omitempty"`
	// +optional
	ClosedReason string `json:"closedReason,omitempty"`
	// +optional
	MergeCommitSHA string `json:"mergeCommitSHA,omitempty"`
	// +optional
	MergedBy string `json:"mergedBy,omitempty"`
	// +optional
	ApprovedBy []string `json:"approvedBy,omitempty"`
	// Targets are the deploy paths the merge affects, copied from
	// status.render at push time.
	// +optional
	Targets []PRTarget `json:"targets,omitempty"`
}

// PRTarget is a deploy path listed on a PR.
type PRTarget struct {
	// +optional
	Name string `json:"name,omitempty"`
	// Namespace is the namespace for rendered objects that have none.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PushFailure says why an approved proposal couldn't be pushed.
type PushFailure struct {
	// Reason is a stable code: BaseTooOld, EditsDontApply, TreeMismatch,
	// TooLarge or ProtectedPath.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Details say which edit failed which check.
	// +optional
	Details []PushFailureDetail `json:"details,omitempty"`
	// At is when the push failed.
	// +optional
	At *metav1.Time `json:"at,omitempty"`
}

// PushFailureDetail names one failed check.
type PushFailureDetail struct {
	// Edit is the edit position.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Edit *int64 `json:"edit,omitempty"`
	// Check is the check that failed.
	// +optional
	Check string `json:"check,omitempty"`
}

// SREProposalList is a list of SREProposal.
// +kubebuilder:object:root=true
type SREProposalList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SREProposal `json:"items"`
}
