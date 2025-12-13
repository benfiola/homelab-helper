package push

import (
	"context"
	"fmt"
	"os"

	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/goccy/go-yaml"
	"github.com/hashicorp/vault-client-go"
)

type Opts struct {
	Address       string
	SecretsPath   string
	StorageBucket string
	StorageToken  string
}

type Pusher struct {
	Vault         *vault.Client
	SecretsPath   string
	StorageBucket string
	StorageToken  string
}

func New(opts *Opts) (*Pusher, error) {
	if opts.SecretsPath == "" {
		return nil, fmt.Errorf("secrets path unset")
	}

	vaultClient, err := vault.New(
		vault.WithAddress(opts.Address),
	)
	if err != nil {
		return nil, err
	}

	pusher := Pusher{
		SecretsPath: opts.SecretsPath,
		Vault:       vaultClient,
	}
	return &pusher, nil
}

func (p *Pusher) Push(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("pushing vault secrets to cloud storage")

	logger.Info("listing kv paths")
	response, err := p.Vault.Secrets.KvV2List(ctx, p.SecretsPath)
	if err != nil {
		return err
	}
	apps := response.Data.Keys

	secrets := map[string]map[string]any{}
	for _, path := range apps {
		logger.Info("get kv path", "path", path)
		fullPath := fmt.Sprintf("%s/%s", p.SecretsPath, path)
		response, err := p.Vault.Secrets.KvV2Read(ctx, fullPath)
		if err != nil {
			return err
		}
		secret := response.Data.Data
		secrets[path] = secret
	}

	logger.Info("write secrets to file")
	secretsBytes, err := yaml.Marshal(secrets)
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	secretsPath := fmt.Sprintf("%s/%s", dir, "secrets-vault.yaml")
	err = os.WriteFile(secretsPath, secretsBytes, 0644)
	if err != nil {
		return err
	}

	logger.Info("upload file to cloud storage")
	return nil
}
