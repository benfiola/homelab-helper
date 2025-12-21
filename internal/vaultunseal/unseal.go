package vaultunseal

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
	if opts.Address == "" {
		return nil, fmt.Errorf("address unset")
	}

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

func (u *Unsealer) WaitForPath(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	for {
		_, err := os.Lstat(u.UnsealKeyPath)
		if err == nil {
			return nil
		}
		logger.Debug("waiting for unseal key file", "path", u.UnsealKeyPath)
		time.Sleep(1 * time.Second)
	}
}

func (u *Unsealer) WaitForVault(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	for {
		_, err := u.Vault.System.SealStatus(ctx)
		if err == nil {
			return nil
		}
		logger.Debug("vault not ready, retrying", "address", u.Address)
		time.Sleep(1 * time.Second)
	}
}

func (u *Unsealer) Unseal(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	logger.Debug("waiting for unseal key file")
	err := u.WaitForPath(ctx)
	if err != nil {
		logger.Error("failed while waiting for unseal key file", "error", err)
		return err
	}

	logger.Debug("waiting for vault to be reachable")
	err = u.WaitForVault(ctx)
	if err != nil {
		logger.Error("failed while waiting for vault", "error", err)
		return err
	}

	logger.Debug("checking vault seal status")
	response, err := u.Vault.System.SealStatus(ctx)
	if err != nil {
		logger.Error("failed to check vault seal status", "error", err)
		return err
	}
	if !response.Data.Sealed {
		logger.Info("vault already unsealed")
		return nil
	}

	logger.Debug("reading unseal key")
	unsealKeyBytes, err := os.ReadFile(u.UnsealKeyPath)
	if err != nil {
		logger.Error("failed to read unseal key file", "path", u.UnsealKeyPath, "error", err)
		return err
	}
	unsealKey := string(unsealKeyBytes)

	logger.Debug("sending unseal request to vault")
	_, err = u.Vault.System.Unseal(ctx, schema.UnsealRequest{Key: unsealKey})
	if err != nil {
		logger.Error("failed to unseal vault", "error", err)
		return err
	}

	logger.Info("vault unsealed successfully")
	return nil
}

func (u *Unsealer) Run(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("starting vault unseal process", "vault", u.Address)

	err := u.Unseal(ctx)
	if err != nil {
		logger.Error("unseal process failed", "error", err)
		return err
	}

	if !u.RunForever {
		return nil
	}

	logger.Info("unseal successful, waiting for shutdown signal")
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGTERM, syscall.SIGINT)
	sig := <-signalChannel
	logger.Info("received signal, shutting down", "signal", sig)

	return nil
}
