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
	Client        *vault.Client
	RunForever    bool
	UnsealKeyPath string
}

func New(opts *Opts) (*Unsealer, error) {
	client, err := vault.New(
		vault.WithAddress(opts.Address),
	)
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
		Address:       opts.Address,
		Client:        client,
		RunForever:    runForever,
		UnsealKeyPath: opts.UnsealKeyPath,
	}
	return &unsealer, nil
}

func (u *Unsealer) Unseal(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("unsealing vault")

	finish := func() error {
		if u.RunForever {
			signalChannel := make(chan os.Signal, 1)
			signal.Notify(signalChannel, syscall.SIGTERM, syscall.SIGINT)
			<-signalChannel
		}
		return nil
	}

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

	logger.Info("wait for vault connectivity", "address", u.Address)
	sealed := false
	for {
		response, err := u.Client.System.SealStatus(ctx)
		if err == nil {
			sealed = response.Data.Sealed
			break
		}
		logger.Debug("vault seal status request failed", "error", err)
		time.Sleep(1 * time.Second)
	}
	if !sealed {
		logger.Info("vault is already unsealed")
		return finish()
	}

	logger.Info("unsealing vault", "address", u.Address)
	_, err := u.Client.System.Unseal(ctx, schema.UnsealRequest{Key: unsealKey})
	if err != nil {
		return err
	}

	logger.Info("vault unsealed", "address", u.Address)
	return finish()
}
