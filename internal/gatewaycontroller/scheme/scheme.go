package scheme

import (
	"github.com/benfiola/homelab-helper/internal/gatewaycontroller/api"
	"k8s.io/apimachinery/pkg/runtime"
	coreapis "k8s.io/client-go/kubernetes/scheme"
	gatewayapis "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func Build() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()

	err := coreapis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = gatewayapis.Install(scheme)
	if err != nil {
		return nil, err
	}

	err = gatewayapisv1a2.Install(scheme)
	if err != nil {
		return nil, err
	}

	err = api.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	return scheme, nil
}
