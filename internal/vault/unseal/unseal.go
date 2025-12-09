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
	RunForever    *bool
	UnsealKeyPath string
}

type Unsealer struct {
	Client        *vaultclient.Client
	RunForever    bool
	UnsealKeyPath string
}

func New(opts *Opts) (*Unsealer, error) {
	client, err := vaultclient.New(&vaultclient.Opts{
		Address: opts.Address,
	})
	if err != nil {
		return nil, err
	}

	runForever := true
	if opts.RunForever != nil {
		runForever = *opts.RunForever
	}

	if opts.UnsealKeyPath == "" {
		return nil, fmt.Errorf("unseal key path unset")
	}

	unsealer := Unsealer{
		Client:        client,
		RunForever:    runForever,
		UnsealKeyPath: opts.UnsealKeyPath,
	}
	return &unsealer, nil
}

func (u *Unsealer) Unseal(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("unsealing vault")

	var unsealKey string
	logger.Info("waiting for unseal key", "unseal-key", u.UnsealKeyPath)
	for {
		unsealKeyBytes, err := os.ReadFile(u.UnsealKeyPath)
		if err == nil {
			unsealKey = string(unsealKeyBytes)
			break
		}
		time.Sleep(1 * time.Second)
	}

	logger.Info("wait for vault connectivity", "address", u.Client.Address)
	unsealed := false
	for {
		status, err := u.Client.Status(ctx)
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

	logger.Info("unsealing vault", "address", u.Client.Address)
	err := u.Client.Unseal(ctx, unsealKey)
	if err != nil {
		return err
	}

	logger.Info("vault unsealed", "address", u.Client.Address)

	if u.RunForever {
		select {}
	}

	return nil
}
