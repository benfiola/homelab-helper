package gatewaycontroller

import (
	"context"

	"github.com/benfiola/homelab-helper/internal/gatewaycontroller/reconciler"
	"github.com/benfiola/homelab-helper/internal/gatewaycontroller/scheme"
	"github.com/benfiola/homelab-helper/internal/logging"
	"k8s.io/client-go/tools/clientcmd"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type Opts struct {
	LeaderElection bool
	HealthAddress  string
	MetricsAddress string
}

type Controller struct {
	HealthAddress  string
	LeaderElection bool
	MetricsAddress string
}

func New(opts *Opts) (*Controller, error) {
	controller := Controller{
		HealthAddress:  opts.HealthAddress,
		LeaderElection: opts.LeaderElection,
		MetricsAddress: opts.MetricsAddress,
	}

	return &controller, nil
}

func (c *Controller) Run(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	logger.Info("getting configuration")
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		logger.Error("failed to get configuration", "error", err)
		return err
	}

	logger.Info("building scheme")
	scheme, err := scheme.Build()
	if err != nil {
		logger.Error("failed to build scheme", "error", err)
		return err
	}

	logger.Info("creating controller manager")
	manager, err := controllerruntime.NewManager(config, controllerruntime.Options{
		BaseContext:            func() context.Context { return ctx },
		HealthProbeBindAddress: c.HealthAddress,
		Metrics:                server.Options{BindAddress: c.MetricsAddress},
		LeaderElection:         c.LeaderElection,
		LeaderElectionID:       "gateway-controller.homelab-helper.benfiola.com",
		Scheme:                 scheme,
	})
	if err != nil {
		logger.Error("failed to create controller manager", "error", err)
		return err
	}

	logger.Info("attaching reconcilers")
	client := manager.GetClient()
	reconcilers := []reconciler.Reconciler{
		&reconciler.WrappedGatewayReconciler{Client: client, Scheme: scheme},
		&reconciler.RouteReconciler{Client: client, Scheme: scheme},
	}
	for index, reconciler := range reconcilers {
		err = reconciler.Register(manager)
		if err != nil {
			logger.Error("failed to register reconciler", "error", err, "index", index)
			return err
		}
	}

	logger.Info("starting controller")
	err = manager.Start(controllerruntime.SetupSignalHandler())
	if err != nil {
		logger.Error("controller manager exited with error", "error", err)
		return err
	}

	return nil
}
