package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Mode is the kill switch. A missing SREConfig means Halted.
// +kubebuilder:validation:Enum=Enabled;ExecutionPaused;Halted
type Mode string

const (
	// ModeEnabled lets everything run.
	ModeEnabled Mode = "Enabled"
	// ModeExecutionPaused stops executors; the agent may still propose.
	ModeExecutionPaused Mode = "ExecutionPaused"
	// ModeHalted stops everything except reading history.
	ModeHalted Mode = "Halted"
)

// SREConfigName is the only name the CRD schema admits.
const SREConfigName = "default"

// SREConfig is the kill switch and the system settings, in one cluster-scoped
// singleton named "default". The chart never creates it: a fresh install has
// none, which every component treats as Halted.
//
// The metadata.name restriction to "default" is added to the generated CRD by
// hack/crdpost, because controller-gen can't put a schema on metadata.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name=Mode,type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name=TriggersPaused,type=boolean,JSONPath=`.spec.triggersPaused`
// +kubebuilder:printcolumn:name=ChangedBy,type=string,JSONPath=`.spec.changedBy`
type SREConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the kill switch and settings.
	// +kubebuilder:default={}
	// +optional
	Spec SREConfigSpec `json:"spec,omitempty"`
}

// SREConfigSpec holds the kill-switch fields first, then the settings.
// Declarative management of settings must omit mode, reason and changedBy.
type SREConfigSpec struct {
	// Mode is the kill switch; defaults to Halted so a bare object starts safe.
	// +kubebuilder:default=Halted
	// +optional
	Mode Mode `json:"mode,omitempty"`
	// Reason is free text for the current mode.
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Reason string `json:"reason,omitempty"`
	// ChangedBy is who last changed the mode; kaddie-ui sets it to the viewer.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	ChangedBy string `json:"changedBy,omitempty"`
	// TriggersPaused stops automatic sessions only.
	// +kubebuilder:default=false
	// +optional
	TriggersPaused bool `json:"triggersPaused,omitempty"`
	// API holds the direct API write path settings.
	// +kubebuilder:default={}
	// +optional
	API APISettings `json:"api,omitempty"`
	// Repositories are the git repositories registered for the GitOps path.
	//
	// MaxItems is not in the spec's sample: without a bound, a 1.28 API
	// server rejects the CRD because the url rule's estimated CEL cost
	// exceeds the per-CRD budget.
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:default={}
	// +listType=map
	// +listMapKey=name
	// +optional
	Repositories []Repository `json:"repositories,omitempty"`
}

// APISettings configures the direct API write path.
type APISettings struct {
	// Enabled says whether Api proposals may be created; the harness re-reads
	// it before creating one. Read-role patches are allowed either way.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// Repository is one registered git repository.
type Repository struct {
	// Name is the key Git proposals refer to.
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`
	// URL is an https or ssh URL with no embedded credentials.
	// +kubebuilder:validation:MaxLength=1024
	// +kubebuilder:validation:XValidation:rule="!self.contains('@') || self.startsWith('ssh://') || self.startsWith('git@')",message="credentials must not appear in the URL"
	URL string `json:"url"`
	// TargetBranch is the branch PRs target.
	// +kubebuilder:validation:MaxLength=255
	TargetBranch string `json:"targetBranch"`
	// ReadablePaths are globs the agent may read.
	// +kubebuilder:default={}
	// +optional
	ReadablePaths []string `json:"readablePaths,omitempty"`
	// EditablePaths are globs proposals may change; empty means reads only.
	// +kubebuilder:default={}
	// +optional
	EditablePaths []string `json:"editablePaths,omitempty"`
	// EditableExtensions are the extensions of files that may be created or edited.
	// +kubebuilder:default={".yaml",".yml",".json"}
	// +optional
	EditableExtensions []string `json:"editableExtensions,omitempty"`
	// ProtectedPaths are extra protected globs that always win over EditablePaths.
	// +kubebuilder:default={}
	// +optional
	ProtectedPaths []string `json:"protectedPaths,omitempty"`
	// PR says how pull requests are opened.
	// +kubebuilder:default={}
	// +optional
	PR PRSettings `json:"pr,omitempty"`
	// DeployTargets are paths deployed by something no reconciler adapter describes.
	// +kubebuilder:default={}
	// +optional
	DeployTargets []DeployTarget `json:"deployTargets,omitempty"`
}

// PRSettings configures how kaddie-git-writer opens pull requests.
type PRSettings struct {
	// BranchPrefix is the fixed prefix of every pushed branch.
	// +kubebuilder:validation:Enum=kaddie/
	// +kubebuilder:default=kaddie/
	// +optional
	BranchPrefix string `json:"branchPrefix,omitempty"`
	// MinRequiredApprovals is how many approvals preflight requires branch
	// protection to demand.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=2
	// +optional
	MinRequiredApprovals *int64 `json:"minRequiredApprovals,omitempty"`
	// Labels are added to every PR.
	// +kubebuilder:default={}
	// +optional
	Labels []string `json:"labels,omitempty"`
	// IncludeJustification says whether the justification goes into the PR body.
	// +kubebuilder:default=true
	// +optional
	IncludeJustification *bool `json:"includeJustification,omitempty"`
	// Attestations are host settings an operator attests to.
	// +kubebuilder:default={}
	// +optional
	Attestations []string `json:"attestations,omitempty"`
}

// DeployTarget is a path deployed by something no reconciler adapter describes.
type DeployTarget struct {
	// Name is shown to reviewers.
	Name string `json:"name"`
	// TrackedRevision is the revision the deployment follows.
	// +optional
	TrackedRevision string `json:"trackedRevision,omitempty"`
	// Path is the repository path it consumes.
	Path string `json:"path"`
	// Tool is how kaddie-renderer renders the path.
	// +optional
	Tool RenderTool `json:"tool,omitempty"`
	// ValueFiles are Helm values files relative to Path, applied in order.
	// +kubebuilder:default={}
	// +optional
	ValueFiles []string `json:"valueFiles,omitempty"`
	// DestinationNamespace is where it deploys.
	DestinationNamespace string `json:"destinationNamespace"`
	// Local says whether it is applied to this cluster.
	Local bool `json:"local"`
}

// SREConfigList is a list of SREConfig.
// +kubebuilder:object:root=true
type SREConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SREConfig `json:"items"`
}
