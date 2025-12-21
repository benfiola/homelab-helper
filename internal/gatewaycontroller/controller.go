package gatewaycontroller

import (
	"context"

	"github.com/benfiola/homelab-helper/internal/logging"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	"k8s.io/client-go/tools/clientcmd"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// +kubebuilder:rbac:groups=gateway-controller.homelab-helper.benfiola.com,resources=wrappedgateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway-controller.homelab-helper.benfiola.com,resources=wrappedgateways/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=create;get;list;update;watch;

type Opts struct {
	LeaderElection bool
	MetricsAddress string
}

type Controller struct {
	LeaderElection bool
	MetricsAddress string
}

func New(opts *Opts) (*Controller, error) {
	controller := Controller{
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
		return err
	}

	logger.Info("creating controller manager")
	manager, err := controllerruntime.NewManager(config, controllerruntime.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			BindAddress: c.MetricsAddress,
		},
		LeaderElection:   c.LeaderElection,
		LeaderElectionID: "gateway-controller.homelab-helper.benfiola.com",
	})
	if err != nil {
		return err
	}

	logger.Info("attaching reconcilers")
	reconciler := Reconciler{}
	err = reconciler.SetupWithManager(manager)
	if err != nil {
		return err
	}

	logger.Info("starting reconciler")
	err = manager.Start(controllerruntime.SetupSignalHandler())
	if err != nil {
		return err
	}

	return nil
}
