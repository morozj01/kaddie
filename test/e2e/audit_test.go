//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// auditEvent holds the audit.k8s.io/v1 Event fields the tests read.
type auditEvent struct {
	AuditID string `json:"auditID"`
	Stage   string `json:"stage"`
	Verb    string `json:"verb"`
	User    struct {
		Username string   `json:"username"`
		Groups   []string `json:"groups"`
	} `json:"user"`
	ImpersonatedUser *struct {
		Username string `json:"username"`
	} `json:"impersonatedUser"`
	ObjectRef *struct {
		Resource    string `json:"resource"`
		Subresource string `json:"subresource"`
		Namespace   string `json:"namespace"`
		Name        string `json:"name"`
		APIGroup    string `json:"apiGroup"`
	} `json:"objectRef"`
	ResponseStatus *struct {
		Code int `json:"code"`
	} `json:"responseStatus"`
	RequestObject json.RawMessage `json:"requestObject"`
}

// auditEvents returns the ResponseComplete events of the cluster's audit
// log that match, polling briefly because the API server writes the log
// asynchronously.
func auditEvents(t *testing.T, match func(auditEvent) bool, want int) []auditEvent {
	t.Helper()
	var found []auditEvent
	deadline := time.Now().Add(20 * time.Second)
	for {
		found = found[:0]
		out, err := runCmd("docker", "exec", cluster+"-control-plane", "cat", "/var/log/kubernetes/audit/audit.log")
		if err != nil {
			t.Fatalf("reading the audit log: %v", err)
		}
		sc := bufio.NewScanner(strings.NewReader(out))
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			var e auditEvent
			if json.Unmarshal(sc.Bytes(), &e) != nil || e.Stage != "ResponseComplete" {
				continue
			}
			if match(e) {
				found = append(found, e)
			}
		}
		if len(found) >= want || time.Now().After(deadline) {
			return found
		}
		time.Sleep(time.Second)
	}
}

// statusWritesTo matches status writes to one proposal.
func statusWritesTo(name string) func(auditEvent) bool {
	return func(e auditEvent) bool {
		return e.ObjectRef != nil && e.ObjectRef.Resource == "sreproposals" && e.ObjectRef.Subresource == "status" &&
			e.ObjectRef.Name == name && (e.Verb == "update" || e.Verb == "patch")
	}
}
