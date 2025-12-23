package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapis "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:object:generate=true
type ListenerTemplate struct {
	Port     gatewayapis.PortNumber        `json:"port"`
	Protocol gatewayapis.ProtocolType      `json:"protocol"`
	TLS      *gatewayapis.GatewayTLSConfig `json:"tls,omitempty"`
}

// +kubebuilder:object:generate=true
type WrappedGatewaySpec struct {
	Addresses        []gatewayapis.GatewaySpecAddress   `json:"addresses,omitempty"`
	AllowedListeners *gatewayapis.AllowedListeners      `json:"allowedListeners,omitempty"`
	BackendTLS       *gatewayapis.GatewayBackendTLS     `json:"backendTLS,omitempty"`
	GatewayClassName gatewayapis.ObjectName             `json:"gatewayClassName"`
	Infrastructure   *gatewayapis.GatewayInfrastructure `json:"infrastructure,omitempty"`
	// +kubebuilder:validation:Required
	ListenerTemplate ListenerTemplate `json:"listenerTemplate"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type WrappedGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              WrappedGatewaySpec   `json:"spec"`
	Status            WrappedGatewayStatus `json:"status"`
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
