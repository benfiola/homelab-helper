package gatewaycontroller

import controllerruntime "sigs.k8s.io/controller-runtime"

type Reconciler struct {
}

func (r *Reconciler) SetupWithManager(mgr controllerruntime.Manager) error {
	return nil
}
