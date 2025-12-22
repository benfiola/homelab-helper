package reconciler

import controllerruntime "sigs.k8s.io/controller-runtime"

type Reconciler interface {
	Register(mgr controllerruntime.Manager) error
}

const (
	AnnotationPreviousParentRefs = "gateway-controller.homelab-helper.benfiola.com/previous-parent-refs"
	AnnotationChildModifiedAt    = "gateway-controller.homelab-helper.benfiola.com/child-modified-at"

	ConditionTypeReady = "Ready"

	Finalizer = "gateway-controller.homelab-helper.benfiola.com/finalizer"

	ReasonFinalizerFailed         = "FinalizerFailed"
	ReasonRoutesFetchFailed       = "RoutesFetchFailed"
	ReasonGatewayFetchFailed      = "GatewayFetchFailed"
	ReasonGatewayStatusFailed     = "GatewayStatusFailed"
	ReasonGatewaySyncFailed       = "GatewaySyncFailed"
	ReasonReconciliationSucceeded = "ReconciliationSucceeded"
)
