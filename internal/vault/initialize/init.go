package initialize

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"slices"

	"github.com/benfiola/homelab-helper/internal/logging"
	vaultclient "github.com/benfiola/homelab-helper/internal/vault/client"
)

//go:embed policy.template.hcl
var policyTemplate string

type Opts struct {
	Address      string
	Roles        []string
	SecretsMount string
	Token        string
}

type Initializer struct {
	Client       *vaultclient.Client
	Roles        []string
	SecretsMount string
}

func New(opts *Opts) (*Initializer, error) {
	if opts.Token == "" {
		return nil, fmt.Errorf("token unset")
	}

	client, err := vaultclient.New(&vaultclient.Opts{
		Address: opts.Address,
		Token:   opts.Token,
	})
	if err != nil {
		return nil, err
	}

	initializer := Initializer{
		Client:       client,
		Roles:        opts.Roles,
		SecretsMount: opts.SecretsMount,
	}
	return &initializer, nil
}

func (i *Initializer) Initialize(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("initializing vault")

	secrets, err := i.Client.ListSecretEngines(ctx)
	if err != nil {
		return err
	}
	if _, ok := secrets[i.SecretsMount]; ok {
		logger.Info("enabling kv2 secrets engine", "path", i.SecretsMount)
		err = i.Client.EnableSecretEngine(ctx, vaultclient.KV2{
			Description: "homelab secrets",
			Path:        i.SecretsMount,
		})
		if err != nil {
			return err
		}
	}

	auths, err := i.Client.ListAuthEngines(ctx)
	if err != nil {
		return err
	}
	if _, ok := auths["kubernetes/"]; !ok {
		logger.Info("enabling kubernetes auth engine")
		err = i.Client.EnableAuthEngine(ctx, "kubernetes")
		if err != nil {
			return err
		}
	}

	logger.Info("configuring kubernetes auth engine")
	key := "KUBERNETES_SERVICE_HOST"
	kHost := os.Getenv(key)
	if kHost == "" {
		return fmt.Errorf("%s unset", key)
	}
	key = "KUBERNETES_SERVICE_PORT"
	kPort := os.Getenv(key)
	if kPort == "" {
		return fmt.Errorf("%s unset", key)
	}
	kCa, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return err
	}
	kJwt, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return err
	}
	data := map[string]string{
		"kubernetes_host":    fmt.Sprintf("https://%s:%s", kHost, kPort),
		"kubernetes_ca_cert": string(kCa),
		"token_reviewer_jwt": string(kJwt),
	}
	err = i.Client.Write(ctx, "auth/kubernetes/config", data)
	if err != nil {
		return err
	}

	policies, err := i.Client.ListPolicies(ctx)
	if err != nil {
		return err
	}

	for _, role := range i.Roles {
		if slices.Contains(policies, role) {
			continue
		}

		logger.Info("creating policy", "role", role)
		policy := fmt.Sprintf(policyTemplate, i.SecretsMount)
		err = i.Client.CreatePolicy(ctx, role, policy)
		if err != nil {
			return err
		}
	}

	for _, role := range i.Roles {
		logger.Info("writing role", "role", role)
		path := fmt.Sprintf("/auth/kubernetes/role/%s", role)
		data := map[string]string{
			"bound_service_account_names":      "*",
			"bound_service_account_namespaces": "*",
			"policies":                         role,
			"ttl":                              "1h",
		}
		err = i.Client.Write(ctx, path, data)
		if err != nil {
			return err
		}
	}

	return nil
}
