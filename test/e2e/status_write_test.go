//go:build e2e

package e2e

import (
	"slices"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/morozj01/kaddie/pkg/apis/v1alpha1"
)

// TestT9_GT6_GT40_StatusWrites covers who may write sreproposals/status and
// the audit trail detection rules D1 and GD1 rely on (implementation plan
// T1.5):
//
//   - T9: a status write by an identity without update on
//     sreproposals/status is denied; every status write, including one that
//     records an outcome or ReadyForPR, appears in the audit log with its
//     real author.
//   - GT6: a status block written by someone other than its intended writer
//     succeeds if RBAC allows it and is audited with its author.
//   - GT40 (API part): a direct write to status.approval by anyone RBAC
//     allows, including a ServiceAccount or an operator naming someone else
//     as actedBy, succeeds and is audited with its real author.
//
// Firing D1/GD1 is the detection queries' job (T6.11, T12.10).
func TestT9_GT6_GT40_StatusWrites(t *testing.T) {
	restoreChart(t)
	installChart(t, "--set", "gitops.enabled=true")
	t.Cleanup(func() { deleteAllProposals(t) })

	p := newGitProposal()
	if err := asServiceAccount(t, "kaddie-tools").Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	key := client.ObjectKeyFromObject(p)

	// write applies mutate to the live proposal's status as c.
	write := func(c client.Client, mutate func(*v1.SREProposalStatus)) error {
		cur := &v1.SREProposal{}
		if err := admin.Get(ctx, key, cur); err != nil {
			return err
		}
		mutate(&cur.Status)
		return c.Status().Update(ctx, cur)
	}

	// T9: identities without update on sreproposals/status are denied.
	denied := map[string]client.Client{
		saName("kaddie-tools"):    asServiceAccount(t, "kaddie-tools"),
		saName("kaddie-sessions"): asServiceAccount(t, "kaddie-sessions"),
		saName("kaddie-agent"):    asServiceAccount(t, "kaddie-agent"),
		saName("kaddie-ui"):       asServiceAccount(t, "kaddie-ui"),
		"oidc:nobody@example.com": asHuman(t, "nobody@example.com"),
	}
	for who, c := range denied {
		err := write(c, func(s *v1.SREProposalStatus) {
			s.Execution = &v1.ExecutionStatus{Phase: "Executed", ActedBy: "mallory@example.com"}
		})
		if !apierrors.IsForbidden(err) {
			t.Errorf("%s wrote status: err = %v, want Forbidden", who, err)
		}
	}

	// Writes RBAC allows, intended writer or not, all succeed.
	opUser := oidcInfo.UsernamePrefix + "operator@example.com"
	allowed := []struct {
		who    string
		c      client.Client
		mutate func(*v1.SREProposalStatus)
	}{
		{saName("kaddie-renderer"), asServiceAccount(t, "kaddie-renderer"), func(s *v1.SREProposalStatus) {
			s.Render = &v1.RenderStatus{Phase: "Rendered", HeadTreeSHA: "6c1f4e7a0d3b6e9c2f5a8d1b4e7c0a3f6d9b2e5c"}
		}},
		// GT6/GT40: a ServiceAccount records ReadyForPR, which only a human
		// should, naming a human as actedBy.
		{saName("kaddie-git-writer"), asServiceAccount(t, "kaddie-git-writer"), func(s *v1.SREProposalStatus) {
			s.Approval = &v1.ApprovalStatus{Phase: "ReadyForPR", ActedBy: "alice@example.com"}
		}},
		{saName("kaddie-git-writer"), asServiceAccount(t, "kaddie-git-writer"), func(s *v1.SREProposalStatus) {
			s.PR = &v1.PRStatus{State: "Open", Branch: "kaddie/" + string(p.UID)}
		}},
		// GT40: an operator writes the approval directly with a wrong actedBy.
		{opUser, operator(t), func(s *v1.SREProposalStatus) {
			s.Approval = &v1.ApprovalStatus{Phase: "Dismissed", ActedBy: "someone-else@example.com"}
		}},
	}
	for _, w := range allowed {
		if err := write(w.c, w.mutate); err != nil {
			t.Errorf("%s writing status: %v", w.who, err)
		}
	}

	// Every write, allowed or denied, is in the audit log with its real author.
	events := auditEvents(t, statusWritesTo(p.Name), len(allowed)+len(denied))
	authors := map[string][]int{}
	for _, e := range events {
		if e.ImpersonatedUser != nil {
			t.Errorf("status write recorded as an impersonation of %s", e.ImpersonatedUser.Username)
		}
		if e.ResponseStatus != nil {
			authors[e.User.Username] = append(authors[e.User.Username], e.ResponseStatus.Code)
		}
	}
	for _, w := range allowed {
		if !slices.Contains(authors[w.who], 200) {
			t.Errorf("audit log has no successful status write by %s (have %v)", w.who, authors)
		}
	}
	for who := range denied {
		if !slices.Contains(authors[who], 403) {
			t.Errorf("audit log has no denied status write by %s (have %v)", who, authors)
		}
	}
}
