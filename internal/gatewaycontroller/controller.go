package gatewaycontroller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/benfiola/homelab-helper/internal/gatewaycontroller/reconciler"
	"github.com/benfiola/homelab-helper/internal/gatewaycontroller/scheme"
	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/clientcmd"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
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

	logger.Info("setting controller-runtime logger")
	crLogger := logr.FromSlogHandler(logger.Handler()).WithName("controller-runtime")
	controllerruntime.SetLogger(crLogger)

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
	webhookServer := webhook.NewServer(webhook.Options{Port: 0})
	manager, err := controllerruntime.NewManager(config, controllerruntime.Options{
		BaseContext:            func() context.Context { return ctx },
		HealthProbeBindAddress: c.HealthAddress,
		LeaderElection:         c.LeaderElection,
		LeaderElectionID:       "gateway-controller.homelab-helper.benfiola.com",
		Metrics:                server.Options{BindAddress: c.MetricsAddress},
		WebhookServer:          webhookServer,
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

	logger.Info("adding probes")
	err = manager.AddHealthzCheck("ping", healthz.Ping)
	if err != nil {
		logger.Error("failed to setup liveness probe", "error", err)
	}
	readyz := func(req *http.Request) error {
		val := manager.GetCache().WaitForCacheSync(req.Context())
		if !val {
			logger.Warn("readyz cache sync check failed")
			return fmt.Errorf("readyz cache sync check failed")
		}
		return nil
	}
	err = manager.AddReadyzCheck("caches", readyz)
	if err != nil {
		logger.Error("failed to setup readiness probe", "error", err)
	}

	logger.Info("starting controller")
	err = manager.Start(controllerruntime.SetupSignalHandler())
	if err != nil {
		logger.Error("controller manager exited with error", "error", err)
		return err
	}

	return nil
}
