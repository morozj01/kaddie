package proposal

import (
	"encoding/json"
	"slices"
	"unicode/utf8"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
)

// proposalNamespace is the only namespace proposals may live in (base §4.1).
const proposalNamespace = "kaddie-system"

// Limits of base §4.1 and §6.1.
const (
	maxRequests  = 20
	maxBodyBytes = 65536
	maxTextChars = 4000
	maxEvidence  = 20
	maxSpecBytes = 262144
)

func malformedFinding(index int, rule string) Finding {
	return Finding{Reason: Malformed, Request: index, Explanation: "The proposal breaks a rule of its format: " + rule + "."}
}

// proposalFindings are the findings about the whole proposal (Request is
// NoRequest): its namespace and the well-formedness rules that aren't about
// one request. Base §4.1 and §6.1 say the API server enforces these too; they
// are re-checked here so no client depends on the CRD schema being right.
func proposalFindings(p *v1alpha1.SREProposal) []Finding {
	var out []Finding
	if p.Namespace != proposalNamespace {
		out = append(out, Finding{Reason: ProposalNamespace, Request: NoRequest,
			Explanation: "The proposal is not in the kaddie-system namespace. Proposals outside it are not covered by the quota and RBAC that guard that namespace."})
	}
	malformed := func(rule string) { out = append(out, malformedFinding(NoRequest, rule)) }
	spec := p.Spec
	switch spec.Type {
	case v1alpha1.ProposalTypeAPI:
		if spec.API == nil || spec.Git != nil {
			malformed("an Api proposal has an api block and no git block")
		}
		if spec.API != nil && (len(spec.API.Requests) < 1 || len(spec.API.Requests) > maxRequests) {
			malformed("an Api proposal has 1 to 20 requests")
		}
	case v1alpha1.ProposalTypeGit:
		if spec.Git == nil || spec.API != nil {
			malformed("a Git proposal has a git block and no api block")
		}
	default:
		malformed("the type is neither Api nor Git")
	}
	if utf8.RuneCountInString(spec.Justification) > maxTextChars {
		malformed("the justification is over 4000 characters")
	}
	if utf8.RuneCountInString(spec.RiskExplanation) > maxTextChars {
		malformed("the risk explanation is over 4000 characters")
	}
	if len(spec.Evidence) > maxEvidence {
		malformed("there are more than 20 evidence references")
	}
	if b, err := json.Marshal(spec); err == nil && len(b) > maxSpecBytes {
		malformed("the spec is over 256 KiB")
	}
	return out
}

// requestMalformed returns one Malformed finding per rule of base §4.1 the
// request breaks.
func requestMalformed(index int, req v1alpha1.RequestFields) []Finding {
	var out []Finding
	malformed := func(rule string) { out = append(out, malformedFinding(index, rule)) }
	if !slices.Contains([]v1alpha1.RequestVerb{v1alpha1.RequestVerbCreate, v1alpha1.RequestVerbPatch, v1alpha1.RequestVerbDelete}, req.Verb) {
		malformed("the verb is not create, patch or delete")
	}
	if (req.Verb == v1alpha1.RequestVerbPatch || req.Verb == v1alpha1.RequestVerbDelete) && req.Name == "" {
		malformed("patch and delete need a name")
	}
	if req.Verb == v1alpha1.RequestVerbPatch && !slices.Contains([]v1alpha1.PatchType{v1alpha1.PatchTypeJSONPatch, v1alpha1.PatchTypeMergePatch, v1alpha1.PatchTypeStrategicMergePatch}, req.PatchType) {
		malformed("patch needs a patch type of JSON Patch, JSON merge patch or strategic merge patch")
	}
	if (req.Verb == v1alpha1.RequestVerbCreate || req.Verb == v1alpha1.RequestVerbPatch) && req.Body == "" {
		malformed("create and patch need a body")
	}
	if len(req.Body) > maxBodyBytes {
		malformed("the body is over 64 KiB")
	}
	if _, err := groupOf(req.APIVersion); err != nil {
		malformed("the apiVersion is neither v1 nor group/version")
	}
	if req.Resource == "" {
		malformed("the resource is missing")
	}
	return out
}
