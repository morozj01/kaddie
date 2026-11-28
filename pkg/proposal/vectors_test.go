package proposal_test

import (
	"slices"
	"testing"

	"github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/pkg/proposal"
	"github.com/morozj01/kaddie/pkg/proposal/conformance"
	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

// vectorsDir holds the shared conformance corpus (T2.7).
const vectorsDir = "../../test/conformance/proposal"

// engine is pkg/proposal itself, with the vendored schemas, as a conformance
// engine.
func engine() conformance.Engine {
	schemas := schema.Vendored()
	return conformance.Engine{
		Static: func(p *v1alpha1.SREProposal) []proposal.Finding { return proposal.EvaluateStatic(p, schemas) },
		Exact: func(p *v1alpha1.SREProposal, observed []proposal.Observed) []proposal.Finding {
			return proposal.EvaluateExact(p, observed, schemas)
		},
	}
}

func loadVectors(t *testing.T) []conformance.Vector {
	t.Helper()
	vectors, err := conformance.Load(vectorsDir)
	if err != nil {
		t.Fatal(err)
	}
	return vectors
}

// runVectors checks every vector tagged with a spec test ID. A tag with no
// vector is a failure, so a test can't pass by running nothing.
func runVectors(t *testing.T, tag string) { runTagged(t, tag, false) }

// runExactVectors is runVectors for the tagged vectors that have observed
// requests, so exact evaluation is checked as well as static.
func runExactVectors(t *testing.T, tag string) { runTagged(t, tag, true) }

func runTagged(t *testing.T, tag string, observedOnly bool) {
	t.Helper()
	ran := 0
	for _, v := range loadVectors(t) {
		if !slices.Contains(v.Tests, tag) || observedOnly && len(v.Observed) == 0 {
			continue
		}
		ran++
		t.Run(v.Name, func(t *testing.T) {
			for _, d := range conformance.Check(v, engine()) {
				t.Error(d)
			}
		})
	}
	if ran == 0 {
		t.Fatalf("no vector is tagged %s", tag)
	}
}

// T6: a proposal in any namespace but kaddie-system is high risk.
func TestT6_ProposalNamespaceVectors(t *testing.T) { runVectors(t, "T6") }

// T11: a proposal created without kaddie-tools is checked for invalid JSON,
// placeholders and invisible characters, and for well-formedness.
func TestT11_DirectCreationVectors(t *testing.T) { runVectors(t, "T11") }

// T12: the match fields of base 4.2 as the catalogue uses them.
func TestT12_CatalogueMatchFieldVectors(t *testing.T) { runVectors(t, "T12") }

// T13: the finding names the request that matched.
func TestT13_FindingNamesTheRequestVectors(t *testing.T) { runVectors(t, "T13") }

// T14: generateName and the namespaces resource can't evade a rule.
func TestT14_GenerateNameAndNamespaceResourceVectors(t *testing.T) { runVectors(t, "T14") }

// T15: every protection of base 6.3, matching and near-miss requests.
func TestT15_ProtectionVectors(t *testing.T) { runVectors(t, "T15") }

// T16: the whole corpus, static and exact, as every consumer of pkg/proposal
// must pass it. Consumers: kaddie-tools (T5.5), kaddie-ui (T6.5, T10.4).
func TestT16_ConformanceVectors(t *testing.T) {
	vectors := loadVectors(t)
	if len(vectors) == 0 {
		t.Fatal("no vectors")
	}
	exact := 0
	for _, v := range vectors {
		if len(v.Observed) > 0 {
			exact++
		}
	}
	if exact == 0 {
		t.Fatal("no vector has observed requests, so exact evaluation isn't checked")
	}
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			for _, d := range conformance.Check(v, engine()) {
				t.Error(d)
			}
		})
	}
}

// T25 (static part): the kaddie-agent-read exception.
func TestT25_AgentReadRoleStaticVectors(t *testing.T) { runVectors(t, "T25") }

// T25 (exact part): the exception holds only when the role the dry run
// returned passes the read-role rule and has no aggregationRule.
func TestT25_ExactReadRole(t *testing.T) { runExactVectors(t, "T25") }

// T40: a protected field changed during the dry run, by the request or by
// admission, is flagged and attributed. (The kaddie-ui preview and
// confirmation that show it: T6.5.)
func TestT40_AdmissionChangesAreFlaggedAndAttributed(t *testing.T) { runExactVectors(t, "T40") }

// T34: a placeholder in a drafted proposal is flagged.
func TestT34_RedactedPlaceholderVectors(t *testing.T) { runVectors(t, "T34") }

// T37: a protected field is flagged on create and on each patch type, an
// unprotected one is not.
func TestT37_CreateAndEachPatchTypeVectors(t *testing.T) { runVectors(t, "T37") }

// T38: ancestors and descendants of a protected path.
func TestT38_SubtreeVectors(t *testing.T) { runVectors(t, "T38") }

// T39: replicas and resources aren't flagged, image and env are.
func TestT39_ReplicasAndResourcesVectors(t *testing.T) { runVectors(t, "T39") }

// T41: field rules never match delete.
func TestT41_DeleteVectors(t *testing.T) { runVectors(t, "T41") }

// TestT15_EveryProtectionHasAMatchingVector fails when a catalogue entry has
// no T15 vector expecting its finding, so a protection added later can't ship
// untested.
func TestT15_EveryProtectionHasAMatchingVector(t *testing.T) {
	expected := map[proposal.Reason]bool{}
	for _, v := range loadVectors(t) {
		if slices.Contains(v.Tests, "T15") {
			for _, e := range v.Static {
				expected[e.Reason] = true
			}
		}
	}
	for _, p := range proposal.Protections() {
		if !expected[p.Reason] {
			t.Errorf("no T15 vector expects %s", p.Reason)
		}
	}
	if !expected[proposal.ReadRoleRule] {
		t.Errorf("no T15 vector expects %s", proposal.ReadRoleRule)
	}
}
