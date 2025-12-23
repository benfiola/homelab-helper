package reconciler

// +kubebuilder:rbac:groups=gateway-controller.homelab-helper.benfiola.com,resources=wrappedgateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway-controller.homelab-helper.benfiola.com,resources=wrappedgateways/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=create;get;list;patch;update;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes,verbs=get;list
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes,verbs=get;list

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/benfiola/homelab-helper/internal/gatewaycontroller/api"
	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/benfiola/homelab-helper/internal/ptr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayapis "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type WrappedGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *WrappedGatewayReconciler) Register(manager controllerruntime.Manager) error {
	return controllerruntime.
		NewControllerManagedBy(manager).
		For(&api.WrappedGateway{}).
		Owns(&gatewayapis.Gateway{}).
		Complete(r)
}

func (r *WrappedGatewayReconciler) Reconcile(pctx context.Context, request controllerruntime.Request) (controllerruntime.Result, error) {
	logger := logging.FromContext(pctx).With("resource", request.NamespacedName)
	ctx := logging.WithLogger(pctx, logger)

	wgateway := api.WrappedGateway{}
	err := r.Get(ctx, request.NamespacedName, &wgateway)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return controllerruntime.Result{}, nil
		}
		logger.Error("failed to fetch wrapped gateway", "error", err)
		return controllerruntime.Result{}, err
	}

	if wgateway.DeletionTimestamp != nil {
		controllerutil.RemoveFinalizer(&wgateway, Finalizer)
		err = r.Update(ctx, &wgateway)
		if err != nil {
			logger.Error("failed to remove finalizer during deletion", "error", err)
			r.setCondition(&wgateway, ReasonFinalizerFailed, err.Error())
			r.Status().Update(ctx, &wgateway)
			return controllerruntime.Result{}, err
		}

		return controllerruntime.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&wgateway, Finalizer) {
		controllerutil.AddFinalizer(&wgateway, Finalizer)
		err = r.Update(ctx, &wgateway)
		if err != nil {
			logger.Error("failed to add finalizer", "error", err)
			r.setCondition(&wgateway, ReasonFinalizerFailed, err.Error())
			r.Status().Update(ctx, &wgateway)
			return controllerruntime.Result{}, err
		}
	}

	spec := gatewayapis.GatewaySpec{
		Addresses:        wgateway.Spec.Addresses,
		BackendTLS:       wgateway.Spec.BackendTLS,
		GatewayClassName: wgateway.Spec.GatewayClassName,
		Infrastructure:   wgateway.Spec.Infrastructure,
		Listeners:        []gatewayapis.Listener{},
	}

	create := false
	gateway := gatewayapis.Gateway{}
	err = r.Get(ctx, request.NamespacedName, &gateway)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error("failed to fetch gateway", "error", err, "namespace", request.Namespace, "name", request.Name)
			r.setCondition(&wgateway, ReasonGatewayFetchFailed, err.Error())
			r.Status().Update(ctx, &wgateway)
			return controllerruntime.Result{}, err
		}

		gateway.Name = wgateway.Name
		gateway.Namespace = wgateway.Namespace
		controllerutil.SetControllerReference(&wgateway, &gateway, r.Scheme)
		create = true
	}

	gateway.Annotations = wgateway.Annotations
	gateway.Labels = wgateway.Labels
	gateway.Spec = spec

	routes, err := r.GetRouteData(ctx, &wgateway)
	if err != nil {
		logger.Error("failed to get route data", "error", err)
		r.setCondition(&wgateway, ReasonRoutesFetchFailed, err.Error())
		r.Status().Update(ctx, &wgateway)
		return controllerruntime.Result{}, err
	}

	for index, route := range routes {
		allowedRoutes := &gatewayapis.AllowedRoutes{
			Namespaces: &gatewayapis.RouteNamespaces{
				From: ptr.Get(gatewayapis.NamespacesFromSelector),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": string(route.Namespace),
					},
				},
			},
			Kinds: []gatewayapis.RouteGroupKind{
				{
					Group: ptr.Get(route.Group),
					Kind:  route.Kind,
				},
			},
		}
		name := gatewayapis.SectionName(fmt.Sprintf("listener-%d", index))
		listener := gatewayapis.Listener{
			AllowedRoutes: allowedRoutes,
			Name:          name,
			Hostname:      ptr.Get(route.Hostname),
			Port:          wgateway.Spec.ListenerTemplate.Port,
			Protocol:      wgateway.Spec.ListenerTemplate.Protocol,
			TLS:           wgateway.Spec.ListenerTemplate.TLS,
		}
		gateway.Spec.Listeners = append(gateway.Spec.Listeners, listener)
	}

	if create {
		err = r.Create(ctx, &gateway)
		if err != nil {
			logger.Error("failed to create gateway", "error", err, "routes", len(routes))
			r.setCondition(&wgateway, ReasonGatewaySyncFailed, err.Error())
			r.Status().Update(ctx, &wgateway)
			return controllerruntime.Result{}, err
		}
	} else {
		err = r.Update(ctx, &gateway)
		if err != nil {
			logger.Error("failed to update gateway", "error", err, "routes", len(routes))
			r.setCondition(&wgateway, ReasonGatewaySyncFailed, err.Error())
			r.Status().Update(ctx, &wgateway)
			return controllerruntime.Result{}, err
		}
	}

	logger.Info("synced gateway", "routes", len(routes))
	wgateway.Status.ObservedGeneration = wgateway.Generation
	wgateway.Status.LastReconciledTime = &metav1.Time{Time: time.Now()}
	r.setCondition(&wgateway, ReasonReconciliationSucceeded, "")
	err = r.Status().Update(ctx, &wgateway)
	if err != nil {
		logger.Error("failed to update wrapped gateway status on success", "error", err)
		r.setCondition(&wgateway, ReasonGatewayStatusFailed, err.Error())
		r.Status().Update(ctx, &wgateway)
		return controllerruntime.Result{}, err
	}

	return controllerruntime.Result{}, nil
}

func (r *WrappedGatewayReconciler) GetNamespace(o client.Object) string {
	namespace := o.GetNamespace()
	if namespace == "" {
		namespace = "default"
	}
	return namespace
}

type RouteData struct {
	Hostname  gatewayapis.Hostname
	Kind      gatewayapis.Kind
	Group     gatewayapis.Group
	Namespace gatewayapis.Namespace
}

func (d RouteData) String() string {
	vals := []string{
		string(d.Hostname),
		string(d.Namespace),
		string(d.Group),
		string(d.Kind),
	}
	val := strings.Join(vals, "/")
	return val
}

func (r *WrappedGatewayReconciler) GetRouteData(ctx context.Context, gateway *api.WrappedGateway) ([]RouteData, error) {
	logger := logging.FromContext(ctx)

	dataMap := map[string]RouteData{}
	addRoute := func(route client.Object, hostnames []gatewayapis.Hostname) {
		if hostnames == nil {
			hostnames = []gatewayapis.Hostname{}
		}
		for _, hostname := range hostnames {
			gvk := route.GetObjectKind().GroupVersionKind()
			group := gatewayapis.Group(gvk.Group)
			kind := gatewayapis.Kind(gvk.Kind)
			namespace := gatewayapis.Namespace(r.GetNamespace(route))
			item := RouteData{
				Group:     group,
				Hostname:  hostname,
				Kind:      kind,
				Namespace: namespace,
			}
			dataMap[item.String()] = item
		}
	}

	httpRoutes := gatewayapis.HTTPRouteList{}
	if err := r.List(ctx, &httpRoutes); err != nil {
		logger.Error("failed to list HTTPRoutes", "error", err)
		return nil, err
	}
	for _, route := range httpRoutes.Items {
		if r.routeReferencesGateway(&route, gateway) {
			addRoute(&route, route.Spec.Hostnames)
		}
	}

	tlsRoutes := gatewayapisv1a2.TLSRouteList{}
	if err := r.List(ctx, &tlsRoutes); err != nil {
		logger.Error("failed to list TLSRoutes", "error", err)
		return nil, err
	}
	for _, route := range httpRoutes.Items {
		if r.routeReferencesGateway(&route, gateway) {
			addRoute(&route, route.Spec.Hostnames)
		}
	}

	grpcRoutes := gatewayapis.GRPCRouteList{}
	if err := r.List(ctx, &grpcRoutes); err != nil {
		logger.Error("failed to list GRPCRoutes", "error", err)
		return nil, err
	}
	for _, route := range httpRoutes.Items {
		if r.routeReferencesGateway(&route, gateway) {
			addRoute(&route, route.Spec.Hostnames)
		}
	}

	keys := []string{}
	for key := range dataMap {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	data := []RouteData{}
	for _, key := range keys {
		data = append(data, dataMap[key])
	}

	return data, nil
}

func (r *WrappedGatewayReconciler) routeReferencesGateway(route client.Object, gateway *api.WrappedGateway) bool {
	var parentRefs []gatewayapis.ParentReference

	switch v := route.(type) {
	case *gatewayapis.HTTPRoute:
		parentRefs = v.Spec.ParentRefs
	case *gatewayapisv1a2.TLSRoute:
		parentRefs = v.Spec.ParentRefs
	case *gatewayapis.GRPCRoute:
		parentRefs = v.Spec.ParentRefs
	default:
		return false
	}

	for _, ref := range parentRefs {
		kind := ""
		if ref.Kind != nil {
			kind = string(*ref.Kind)
		}

		name := string(ref.Name)

		namespace := gateway.Namespace
		if ref.Namespace != nil {
			namespace = string(*ref.Namespace)
		}

		if kind == "Gateway" && namespace == gateway.Namespace && name == gateway.Name {
			return true
		}
	}

	return false
}

func (r *WrappedGatewayReconciler) setCondition(wg *api.WrappedGateway, reason string, message string) {
	cstatus := metav1.ConditionFalse
	if reason == ReasonReconciliationSucceeded {
		cstatus = metav1.ConditionTrue
	}

	meta.SetStatusCondition(&wg.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeReady,
		Status:             cstatus,
		ObservedGeneration: wg.Generation,
		Reason:             reason,
		Message:            message,
	})
}
