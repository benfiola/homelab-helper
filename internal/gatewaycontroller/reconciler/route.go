package reconciler

// +kubebuilder:rbac:groups=gateway-controller.homelab-helper.benfiola.com,resources=wrappedgateways,verbs=list;patch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes,verbs=get;list;patch;update;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;patch;update;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes,verbs=get;list;patch;update;watch

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/benfiola/homelab-helper/internal/gatewaycontroller/api"
	"github.com/benfiola/homelab-helper/internal/logging"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayapis "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type RouteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *RouteReconciler) Register(manager controllerruntime.Manager) error {
	resources := []client.Object{
		&gatewayapis.GRPCRoute{},
		&gatewayapis.HTTPRoute{},
		&gatewayapisv1a2.TLSRoute{},
	}

	for _, resource := range resources {
		err := controllerruntime.NewControllerManagedBy(manager).For(resource).Complete(r)
		if err != nil {
			return err
		}
	}

	return nil
}

type RouteParentRefs struct {
	ParentRefs []gatewayapis.ParentReference `json:",inline"`
}

func (r *RouteReconciler) setPreviousParentRefs(_ context.Context, route client.Object, refs []gatewayapis.ParentReference) {
	data := RouteParentRefs{ParentRefs: refs}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return
	}

	if route.GetAnnotations() == nil {
		route.SetAnnotations(map[string]string{})
	}
	route.GetAnnotations()[AnnotationPreviousParentRefs] = string(dataBytes)
}

func (r *RouteReconciler) getPreviousParentRefs(_ context.Context, route client.Object) []gatewayapis.ParentReference {
	annotations := route.GetAnnotations()
	if annotations == nil {
		return []gatewayapis.ParentReference{}
	}

	dataStr, ok := annotations[AnnotationPreviousParentRefs]
	if !ok {
		return []gatewayapis.ParentReference{}
	}

	data := RouteParentRefs{}
	err := json.Unmarshal([]byte(dataStr), &data)
	if err != nil {
		return []gatewayapis.ParentReference{}
	}

	return data.ParentRefs
}

func (r *RouteReconciler) triggerReconcile(ctx context.Context, route client.Object, refs []gatewayapis.ParentReference) error {
	logger := logging.FromContext(ctx)

	wrappedGateways := map[string]api.WrappedGateway{}
	for _, ref := range refs {
		group := "gateway.networking.k8s.io"
		if ref.Group != nil {
			group = string(*ref.Group)
		}

		kind := "Gateway"
		if ref.Kind != nil {
			kind = string(*ref.Kind)
		}

		if group != "gateway.networking.k8s.io" || kind != "Gateway" {
			continue
		}

		namespace := route.GetNamespace()
		if ref.Namespace != nil {
			namespace = string(*ref.Namespace)
		}
		name := string(ref.Name)
		key := fmt.Sprintf("%s/%s", namespace, name)
		_, seen := wrappedGateways[key]
		if seen {
			continue
		}

		wrappedGateway := api.WrappedGateway{}
		err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &wrappedGateway)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error("failed to get wrapped gateway", "error", err, "namespace", namespace, "name", name)
				return err
			}
			continue
		}

		wrappedGateways[key] = wrappedGateway
	}

	now := time.Now().Format(time.RFC3339)
	for _, wrappedGateway := range wrappedGateways {
		if wrappedGateway.Annotations == nil {
			wrappedGateway.Annotations = map[string]string{}
		}
		wrappedGateway.Annotations[AnnotationChildModifiedAt] = now
		err := r.Update(ctx, &wrappedGateway)
		if err != nil {
			logger.Error("failed to update wrapped gateway annotation", "error", err, "namespace", wrappedGateway.Namespace, "name", wrappedGateway.Name)
			return err
		}
	}

	return nil
}

func (r *RouteReconciler) ReconcileRoute(ctx context.Context, route client.Object) (controllerruntime.Result, error) {
	logger := logging.FromContext(ctx)

	if route.GetDeletionTimestamp() != nil {
		previous := r.getPreviousParentRefs(ctx, route)

		err := r.triggerReconcile(ctx, route, previous)
		if err != nil {
			logger.Error("failed to trigger reconciliation of parent refs", "error", err)
			return controllerruntime.Result{}, err
		}

		controllerutil.RemoveFinalizer(route, Finalizer)
		err = r.Update(ctx, route)
		if err != nil {
			logger.Error("failed to remove finalizer during deletion", "error", err)
			return controllerruntime.Result{}, err
		}

		return controllerruntime.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(route, Finalizer) {
		controllerutil.AddFinalizer(route, Finalizer)
		err := r.Update(ctx, route)
		if err != nil {
			logger.Error("failed to add finalizer", "error", err)
			return controllerruntime.Result{}, err
		}
	}

	previous := r.getPreviousParentRefs(ctx, route)
	current := []gatewayapis.ParentReference{}
	switch v := route.(type) {
	case *gatewayapis.HTTPRoute:
		current = v.Spec.ParentRefs
	case *gatewayapis.GRPCRoute:
		current = v.Spec.ParentRefs
	case *gatewayapisv1a2.TLSRoute:
		current = v.Spec.ParentRefs
	}
	if slices.Equal(previous, current) {
		return controllerruntime.Result{}, nil
	}

	refs := []gatewayapis.ParentReference{}
	refs = append(refs, previous...)
	refs = append(refs, current...)
	err := r.triggerReconcile(ctx, route, refs)
	if err != nil {
		logger.Error("failed to trigger reconciliation of parent refs", "error", err)
		return controllerruntime.Result{}, err
	}

	r.setPreviousParentRefs(ctx, route, current)
	err = r.Update(ctx, route)
	if err != nil {
		logger.Error("failed to update resource", "error", err)
		return controllerruntime.Result{}, err
	}

	return controllerruntime.Result{}, nil
}

func (r *RouteReconciler) Reconcile(pctx context.Context, request controllerruntime.Request) (controllerruntime.Result, error) {
	logger := logging.FromContext(pctx).With("resource", request.NamespacedName)
	ctx := logging.WithLogger(pctx, logger)

	u := unstructured.Unstructured{}
	err := r.Get(ctx, request.NamespacedName, &u)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return controllerruntime.Result{}, nil
		}
		logger.Error("failed to fetch route", "error", err)
		return controllerruntime.Result{}, err
	}

	gvk := u.GroupVersionKind()
	obj, err := r.Scheme.New(gvk)
	if err != nil {
		logger.Error("unknown gvk", "error", err, "gvk", gvk.String())
		return controllerruntime.Result{}, nil
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
	if err != nil {
		logger.Error("failed to convert unstructured route", "error", err, "gvk", gvk.String())
		return controllerruntime.Result{}, nil
	}

	route, ok := obj.(client.Object)
	if !ok {
		logger.Error("route does not implement client.Object", "gvk", gvk.String())
		return controllerruntime.Result{}, nil
	}

	return r.ReconcileRoute(ctx, route)
}
