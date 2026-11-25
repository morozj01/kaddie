package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TriggerMode is Shadow (record fires only) or Active (start sessions).
// +kubebuilder:validation:Enum=Shadow;Active
type TriggerMode string

const (
	// TriggerModeShadow records each fire on its trigger instances only.
	TriggerModeShadow TriggerMode = "Shadow"
	// TriggerModeActive starts or resumes sessions.
	TriggerModeActive TriggerMode = "Active"
)

// SRETrigger turns something the system can observe into a session, or into
// a new run in an existing session. Cluster-scoped and operator-managed.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name=Mode,type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name=Valid,type=string,JSONPath=`.status.conditions[?(@.type=='Valid')].status`
// +kubebuilder:printcolumn:name=Firing,type=integer,JSONPath=`.status.firingInstances`
// +kubebuilder:printcolumn:name=FiredToday,type=integer,JSONPath=`.status.firedToday`
type SRETrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the trigger definition, written by operators.
	Spec SRETriggerSpec `json:"spec"`
	// Status is written only by kaddie-sessions' trigger evaluator.
	// +optional
	Status SRETriggerStatus `json:"status,omitempty"`
}

// SRETriggerSpec has exactly one source: watch, schedule or webhook.
//
// +kubebuilder:validation:XValidation:rule="[has(self.watch), has(self.schedule), has(self.webhook)].filter(x, x).size() == 1",message="exactly one of watch, schedule or webhook must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.session.resume) || self.session.resume != 'ProposalSession' || (has(self.watch) && self.watch.resource == 'sreproposals')",message="ProposalSession resume requires a watch on sreproposals"
type SRETriggerSpec struct {
	// Mode defaults to Shadow: new triggers only record what they would do
	// until promoted.
	// +kubebuilder:default=Shadow
	// +optional
	Mode TriggerMode `json:"mode,omitempty"`
	// Watch fires on a CEL condition over a watched object.
	// +optional
	Watch *WatchSource `json:"watch,omitempty"`
	// Schedule fires on a cron schedule.
	// +optional
	Schedule *ScheduleSource `json:"schedule,omitempty"`
	// Webhook fires on an authenticated POST; works only when webhooks are
	// enabled in Helm values.
	// +optional
	Webhook *WebhookSource `json:"webhook,omitempty"`
	// Session is the session to start or resume.
	Session TriggerSession `json:"session"`
	// Controls holds the trigger's one control, cooldown.
	// +optional
	Controls *TriggerControls `json:"controls,omitempty"`
}

// WatchSource watches a resource with the agent's own read role.
type WatchSource struct {
	// APIVersion is the group/version of the watched kind.
	APIVersion string `json:"apiVersion"`
	// Resource is the watched resource, readable only through the agent's
	// read role.
	Resource string `json:"resource"`
	// NamespaceSelector optionally limits which namespaces are watched.
	// +optional
	NamespaceSelector *NamespaceSelector `json:"namespaceSelector,omitempty"`
	// When is CEL over object and now, compiled with a cost limit.
	// +kubebuilder:validation:MaxLength=4096
	When string `json:"when"`
	// For is how long the condition must hold; a flap resets it.
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	// +optional
	For string `json:"for,omitempty"`
	// Subject is optional CEL over object returning {apiVersion, kind,
	// namespace, name}, such as the pod's owner; it keys the trigger instance.
	// +kubebuilder:validation:MaxLength=4096
	// +optional
	Subject string `json:"subject,omitempty"`
}

// NamespaceSelector is a standard label selector over namespaces.
type NamespaceSelector struct {
	// MatchLabels are exact label matches.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	// MatchExpressions are set-based label matches.
	// +optional
	MatchExpressions []SelectorRequirement `json:"matchExpressions,omitempty"`
}

// SelectorOperator is a label selector operator.
// +kubebuilder:validation:Enum=In;NotIn;Exists;DoesNotExist
type SelectorOperator string

// SelectorRequirement is one set-based label match.
type SelectorRequirement struct {
	// +optional
	Key string `json:"key,omitempty"`
	// +optional
	Operator SelectorOperator `json:"operator,omitempty"`
	// +optional
	Values []string `json:"values,omitempty"`
}

// ScheduleSource fires with no object, on one trigger instance per mode.
type ScheduleSource struct {
	// Cron is a five-field cron expression.
	// +kubebuilder:validation:MaxLength=100
	Cron string `json:"cron"`
	// Timezone is the IANA time zone the expression is evaluated in.
	// +kubebuilder:default=UTC
	// +optional
	Timezone string `json:"timezone,omitempty"`
}

// WebhookSource is served as POST /webhooks/<path> on kaddie-sessions'
// dedicated webhook port.
type WebhookSource struct {
	// Path is served as POST /webhooks/<path>.
	// +kubebuilder:validation:Pattern=`^[a-z0-9-]{1,63}$`
	Path string `json:"path"`
	// Auth says how senders authenticate.
	Auth WebhookAuth `json:"auth"`
	// When is CEL over the parsed payload; a false result for an instance's
	// name clears it.
	// +kubebuilder:validation:MaxLength=4096
	When string `json:"when"`
	// Subject is CEL over the payload returning the name that keys the
	// trigger instance.
	// +kubebuilder:validation:MaxLength=4096
	// +kubebuilder:default=payload.name
	// +optional
	Subject string `json:"subject,omitempty"`
	// ExpireAfter clears a firing instance this long after it fired if no
	// delivery cleared it.
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	// +kubebuilder:default=24h
	// +optional
	ExpireAfter string `json:"expireAfter,omitempty"`
}

// WebhookAuthType is how a webhook sender authenticates.
// +kubebuilder:validation:Enum=Bearer;HMAC
type WebhookAuthType string

// WebhookAuth names the Secret holding the token or HMAC key.
type WebhookAuth struct {
	// Type is Bearer or HMAC (HMAC-SHA256 over the body).
	Type WebhookAuthType `json:"type"`
	// SecretName is a Secret in kaddie-system holding the token or key.
	SecretName string `json:"secretName"`
}

// ResumeMode says whether a fire starts a new session or resumes the
// proposal's own session.
// +kubebuilder:validation:Enum=None;ProposalSession
type ResumeMode string

// TriggerSession is the session a trigger starts or resumes.
type TriggerSession struct {
	// Goal is fixed operator-written text the agent receives as signal data.
	// +kubebuilder:validation:MaxLength=2000
	// +optional
	Goal string `json:"goal,omitempty"`
	// Resume starts a new session (None) or resumes the proposal's session.
	// +kubebuilder:default=None
	// +optional
	Resume ResumeMode `json:"resume,omitempty"`
}

// TriggerControls holds the trigger's one control.
type TriggerControls struct {
	// Cooldown is the quiet period after an instance last fired; a held-back
	// fire is retried later.
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	// +optional
	Cooldown string `json:"cooldown,omitempty"`
}

// SRETriggerStatus is written only by kaddie-sessions' trigger evaluator.
type SRETriggerStatus struct {
	// LastFired is the last fire of any of its instances.
	// +optional
	LastFired *metav1.Time `json:"lastFired,omitempty"`
	// Instances counts trigger instances in the current mode.
	// +optional
	Instances *int64 `json:"instances,omitempty"`
	// FiringInstances counts instances currently firing.
	// +optional
	FiringInstances *int64 `json:"firingInstances,omitempty"`
	// FiredToday counts sessions started today (Shadow: fires recorded).
	// +optional
	FiredToday *int64 `json:"firedToday,omitempty"`
	// SuppressedToday counts matches that started nothing, by reason.
	// +optional
	SuppressedToday map[string]int64 `json:"suppressedToday,omitempty"`
	// Conditions is the standard condition list.
	// +optional
	Conditions []TriggerCondition `json:"conditions,omitempty"`
}

// ConditionStatus is True, False or Unknown.
// +kubebuilder:validation:Enum="True";"False";"Unknown"
type ConditionStatus string

// TriggerCondition is one condition, such as Valid.
type TriggerCondition struct {
	// Type is the condition type, such as Valid.
	Type string `json:"type"`
	// Status is True, False or Unknown.
	Status ConditionStatus `json:"status"`
	// Reason is machine-readable.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is human-readable.
	// +optional
	Message string `json:"message,omitempty"`
}

// SRETriggerList is a list of SRETrigger.
// +kubebuilder:object:root=true
type SRETriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SRETrigger `json:"items"`
}
