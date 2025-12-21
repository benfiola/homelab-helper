package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapis "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type WrappedGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   gatewayapis.GatewaySpec `json:"spec,omitempty"`
	Status WrappedGatewayStatus    `json:"status,omitempty"`
}

type WrappedGatewayStatus struct {
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
type WrappedGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WrappedGateway `json:"items"`
}
