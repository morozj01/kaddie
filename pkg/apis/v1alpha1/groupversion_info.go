package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Group is the API group of every Kaddie custom resource.
const Group = "kaddie.org"

var (
	// GroupVersion is the group and version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: Group, Version: "v1alpha1"}

	// SchemeBuilder registers the types with a runtime.Scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion,
		&SREProposal{}, &SREProposalList{},
		&SREConfig{}, &SREConfigList{},
		&SRETrigger{}, &SRETriggerList{},
	)
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}
