package proposal

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

// ProtectionsVersion names this catalogue. It changes whenever a protection,
// a reason code or a rendering changes.
const ProtectionsVersion = "1"

// Protection is one entry of the built-in catalogue (base §6.3).
type Protection struct {
	Reason Reason
	// Group is the catalogue heading: System integrity, Sensitive data, What
	// code runs and as whom, Deployment mechanism, Previewability.
	Group string
	// Summary is the one-line match summary of the policy summary.
	Summary string
	// Explanation says what a matching change could do.
	Explanation string
	// Rules match when any one of them does.
	Rules []Rule
}

// Protections returns a copy of the built-in catalogue, in rendering order.
func Protections() []Protection {
	return cloneProtections(catalogue)
}

// RenderPolicy renders the catalogue as the policy summary's protection
// lines (harness B.3), "<group>: <summary>", one per protection, in catalogue
// order, deterministically.
func RenderPolicy() string {
	var b strings.Builder
	for _, p := range catalogue {
		b.WriteString(p.Group + ": " + p.Summary + "\n")
	}
	return b.String()
}

// EvaluateStatic evaluates a proposal from its own content, with no cluster
// access (base §6.2): well-formedness, the proposal namespace, body hygiene,
// and the built-in protections with field rules evaluated on changes.Static.
// The kaddie-agent-read exception applies only to a patch whose every
// operation adds, replaces or removes whole rules, each passing the
// read-role rule (base §5.4.4). No findings means not high risk. Findings are
// ordered: the proposal's own (NoRequest) first, then by request, and within
// a request Malformed, body hygiene, then catalogue order. Git proposals get
// the proposal checks only until pkg/gitproposal's static checks land (T12.2).
func EvaluateStatic(p *v1alpha1.SREProposal, schemas schema.Provider) []Finding {
	return evaluateStatic(p, schemas)
}

// Observed is what an executor saw for one request during preview.
type Observed struct {
	// Request is the index in spec.api.requests.
	Request int `json:"request"`
	// Live is the object before the request; empty for create.
	Live json.RawMessage `json:"live,omitempty"`
	// DryRun is the object the server-side dry run returned.
	DryRun json.RawMessage `json:"dryRun"`
}

// EvaluateExact evaluates the built-in protections against what each
// observed request really changes (base §4.2.1 item 2). For a patch, the
// request's own changes are changes.Diff(Live, Apply(Live, patch)) and
// admission's are the rest of changes.Diff(Live, DryRun); a finding names
// SourceRequest when an own change matches, otherwise SourceAdmission. For
// create, own changes are the body's fields and admission's are body fields
// whose dry-run value differs, so server-applied defaults don't count. The
// kaddie-agent-read exception holds only when the DryRun role passes the
// read-role rule. Requests without an Observed entry are skipped.
func EvaluateExact(p *v1alpha1.SREProposal, observed []Observed, schemas schema.Provider) []Finding {
	return evaluateExact(p, observed, schemas)
}

// RefusalReason is a stable code for a failed acting precondition.
type RefusalReason string

const (
	// Halted: SREConfig/default is missing, unreadable, or in mode Halted.
	Halted RefusalReason = "Halted"
	// ExecutionPaused: mode is ExecutionPaused.
	ExecutionPaused RefusalReason = "ExecutionPaused"
	// NotOpen: the proposal already has an outcome.
	NotOpen RefusalReason = "NotOpen"
	// Expired: an expiry is configured and has passed.
	Expired RefusalReason = "Expired"
)

// Refusal is one failed acting precondition.
type Refusal struct {
	Reason      RefusalReason
	Explanation string
}

// CanAct reports every acting precondition the proposal fails (base §7.2):
// mode Enabled (a nil cfg is a missing SREConfig, so Halted), open, and
// unexpired, where expiry is proposals.expirySeconds and zero never expires.
// An empty result means a human may act. Risk findings are deliberately not
// an input: they change how a proposal is shown, never whether it can be
// acted on.
func CanAct(cfg *v1alpha1.SREConfig, p *v1alpha1.SREProposal, expiry time.Duration, now time.Time) []Refusal {
	panic("unimplemented")
}
