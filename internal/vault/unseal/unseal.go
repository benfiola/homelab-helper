package unseal

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

type Opts struct {
	Address       string
	RunForever    *bool
	UnsealKeyPath string
}

type Unsealer struct {
	Address       string
	Vault         *vault.Client
	RunForever    bool
	UnsealKeyPath string
}

func New(opts *Opts) (*Unsealer, error) {

	runForever := true
	if opts.RunForever != nil {
		runForever = *opts.RunForever
	}

	if opts.UnsealKeyPath == "" {
		return nil, fmt.Errorf("unseal key path unset")
	}

	vaultClient, err := vault.New(
		vault.WithAddress(opts.Address),
	)
	if err != nil {
		return nil, err
	}

	unsealer := Unsealer{
		Address:       opts.Address,
		RunForever:    runForever,
		UnsealKeyPath: opts.UnsealKeyPath,
		Vault:         vaultClient,
	}
	return &unsealer, nil
}

func (u *Unsealer) WaitForPath(ctx context.Context, path string) {
	for {
		_, err := os.Lstat(path)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}
}

func (u *Unsealer) WaitForVault(ctx context.Context, address string) {
	logger := logging.FromContext(ctx)

	for {
		_, err := u.Vault.System.SealStatus(ctx)
		if err != nil {
			logger.Debug("vault seal status request failed", "error", err)
			time.Sleep(1 * time.Second)
			break
		}
		break
	}
}

func (u *Unsealer) Unseal(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("unsealing vault")

	logger.Info("waiting for unseal key", "unseal-key-path", u.UnsealKeyPath)
	u.WaitForPath(ctx, u.UnsealKeyPath)

	logger.Info("waiting for vault", "address", u.Address)
	u.WaitForVault(ctx, u.Address)

	response, err := u.Vault.System.SealStatus(ctx)
	if err != nil {
		return err
	}
	if !response.Data.Sealed {
		logger.Info("vault unsealed")
		return nil
	}

	logger.Info("reading unseal key", "unseal-key-path", u.UnsealKeyPath)
	unsealKeyBytes, err := os.ReadFile(u.UnsealKeyPath)
	if err != nil {
		return err
	}
	unsealKey := string(unsealKeyBytes)

	logger.Info("unsealing vault", "address", u.Address)
	_, err = u.Vault.System.Unseal(ctx, schema.UnsealRequest{Key: unsealKey})
	if err != nil {
		return err
	}

	logger.Info("vault unsealed", "address", u.Address)
	return nil
}

func (u *Unsealer) Run(ctx context.Context) error {
	err := u.Unseal(ctx)
	if err != nil {
		return err
	}

	if u.RunForever {
		signalChannel := make(chan os.Signal, 1)
		signal.Notify(signalChannel, syscall.SIGTERM, syscall.SIGINT)
		<-signalChannel
	}

	return nil
}
