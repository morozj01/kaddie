package proposal

// Reason is a stable code for a risk finding. The codes are metric label
// values and UI anchors, so they never change meaning once released.
type Reason string

// Proposal checks (base §6.2).
const (
	// Malformed: the proposal breaks a rule of base §4.1 and §6.1, re-checked
	// so no client depends on the CRD schema being installed correctly.
	Malformed Reason = "Malformed"
	// ProposalNamespace: the proposal lives outside kaddie-system.
	ProposalNamespace Reason = "ProposalNamespace"
)

// Body hygiene (base §6.2), per request.
const (
	// InvalidJSON: the body isn't valid JSON of the kind its verb and patch
	// type require.
	InvalidJSON Reason = "InvalidJSON"
	// RedactedPlaceholder: a decoded key or string contains "<redacted:".
	RedactedPlaceholder Reason = "RedactedPlaceholder"
	// InvisibleCharacter: the body, or a decoded key or string, holds a
	// Unicode Cf or Co character, a leading byte-order mark included.
	InvisibleCharacter Reason = "InvisibleCharacter"
)

// Built-in protections (base §6.3), one per catalogue entry.
const (
	KaddieGroup              Reason = "KaddieGroup"
	KaddieNamespace          Reason = "KaddieNamespace"
	SystemNamespace          Reason = "SystemNamespace"
	RBAC                     Reason = "RBAC"
	ReadRoleRule             Reason = "ReadRoleRule"
	AdmissionControl         Reason = "AdmissionControl"
	CustomResourceDefinition Reason = "CustomResourceDefinition"
	APIService               Reason = "APIService"
	ServiceAccountToken      Reason = "ServiceAccountToken"
	CSRApproval              Reason = "CSRApproval"
	Secret                   Reason = "Secret"
	SecretDeliveryKind       Reason = "SecretDeliveryKind"
	WorkloadField            Reason = "WorkloadField"
	ReconcilerKind           Reason = "ReconcilerKind"
	DeleteCollection         Reason = "DeleteCollection"
	ConnectSubresource       Reason = "ConnectSubresource"
)

// Source says what caused an exact finding's change.
type Source string

const (
	// SourceRequest: the request's own change, found by applying it
	// client-side. Static findings leave Source empty.
	SourceRequest Source = "Request"
	// SourceAdmission: a change admission (a mutating webhook or defaulting)
	// made during the dry run that the request itself didn't.
	SourceAdmission Source = "Admission"
)

// NoRequest is Finding.Request for a finding about the whole proposal.
const NoRequest = -1

// Finding is one reason a proposal is high risk. It never carries request
// bodies or values from them: only codes, indexes, field paths and fixed text.
type Finding struct {
	Reason Reason `json:"reason"`
	// Request is the index in spec.api.requests, or NoRequest.
	Request int `json:"request"`
	// Field is the protected rule path that matched, for field rules.
	Field string `json:"field,omitempty"`
	// Changed is the changed field that matched Field, in fieldpath.Format.
	Changed string `json:"changed,omitempty"`
	// Source is set on exact findings only.
	Source Source `json:"source,omitempty"`
	// Explanation says in plain language what matched and what the change
	// could do.
	Explanation string `json:"explanation"`
}
