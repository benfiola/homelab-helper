package unseal

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/benfiola/homelab-helper/internal/logging"
	vaultclient "github.com/benfiola/homelab-helper/internal/vault/client"
)

type Opts struct {
	Address       string
	UnsealKeyPath string
}

type Unsealer struct {
	Address       string
	UnsealKeyPath string
}

func New(opts *Opts) (*Unsealer, error) {
	if opts.UnsealKeyPath == "" {
		return nil, fmt.Errorf("unseal key path unset")
	}

	unsealer := Unsealer{
		Address:       opts.Address,
		UnsealKeyPath: opts.UnsealKeyPath,
	}
	return &unsealer, nil
}

func (u *Unsealer) Unseal(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("unsealing vault")

	vault, err := vaultclient.New(&vaultclient.Opts{
		Address: u.Address,
	})
	if err != nil {
		return err
	}

	unsealKeyPath := "/vault/data/unseal-key"
	var unsealKey string
	logger.Info("waiting for vault unseal key")
	for {
		unsealKeyBytes, err := os.ReadFile(unsealKeyPath)
		if err == nil {
			unsealKey = string(unsealKeyBytes)
			break
		}
		time.Sleep(1 * time.Second)
	}

	logger.Info("wait for vault connectivity")
	unsealed := false
	for {
		status, err := vault.Status(ctx)
		if err == nil {
			unsealed = status.Unsealed
			break
		}
		time.Sleep(1 * time.Second)
	}
	if unsealed {
		logger.Info("vault is already unsealed")
		return nil
	}

	logger.Info("unsealing vault")
	err = vault.Unseal(ctx, unsealKey)
	if err != nil {
		return err
	}

	return nil
}
