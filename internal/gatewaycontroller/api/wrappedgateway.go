package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapis "sigs.k8s.io/gateway-api/apis/v1"
)

type WrappedGatewaySpec struct {
	gatewayapis.GatewaySpec `json:",inline"`
	// +kubebuilder:validation:Required
	ListenerTemplate gatewayapis.Listener `json:"listenerTemplate"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type WrappedGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WrappedGatewaySpec   `json:"spec"`
	Status            WrappedGatewayStatus `json:"status"`
}

func (in *WrappedGatewaySpec) DeepCopyInto(out *WrappedGatewaySpec) {
	*out = *in
	in.GatewaySpec.DeepCopyInto(&out.GatewaySpec)
	in.ListenerTemplate.DeepCopyInto(&out.ListenerTemplate)
}

func (in *WrappedGatewaySpec) DeepCopy() *WrappedGatewaySpec {
	if in == nil {
		return nil
	}
	out := new(WrappedGatewaySpec)
	in.DeepCopyInto(out)
	return out
}

// +kubebuilder:object:generate=true
type WrappedGatewayStatus struct {
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	LastReconciledTime *metav1.Time       `json:"lastReconciledTime,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
type WrappedGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []WrappedGateway `json:"items"`
}
