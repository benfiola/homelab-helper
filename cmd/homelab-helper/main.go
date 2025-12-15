package main

import (
	"context"
	"fmt"
	"os"

	"github.com/benfiola/homelab-helper/internal/info"
	"github.com/benfiola/homelab-helper/internal/linstor/diskprovisioner"
	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/benfiola/homelab-helper/internal/ptr"
	"github.com/benfiola/homelab-helper/internal/vault/push"
	"github.com/benfiola/homelab-helper/internal/vault/unseal"
	"github.com/urfave/cli/v3"
)

func main() {
	cli.VersionPrinter = func(cmd *cli.Command) {
		fmt.Fprint(cmd.Root().Writer, cmd.Root().Version)
	}

	command := &cli.Command{
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			format := c.String("log-format")
			level := c.String("log-level")
			logger, err := logging.New(&logging.Opts{Format: format, Level: level})
			if err != nil {
				return ctx, err
			}

			sctx := logging.WithLogger(ctx, logger)
			return sctx, nil
		},
		Commands: []*cli.Command{
			{
				Name: "linstor-provision-disk",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "partition-label",
						Required: true,
						Sources:  cli.EnvVars("PARTITION_LABEL"),
					},
					&cli.StringFlag{
						Name:     "pool",
						Required: true,
						Sources:  cli.EnvVars("POOL"),
					},
					&cli.StringFlag{
						Name:     "volume-group",
						Required: true,
						Sources:  cli.EnvVars("VOLUME_GROUP"),
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					partitionLabel := c.String("partition-label")
					pool := c.String("pool")
					volumeGroup := c.String("volume-group")

					provisioner, err := diskprovisioner.New(&diskprovisioner.Opts{
						PartitionLabel: partitionLabel,
						Pool:           pool,
						VolumeGroup:    volumeGroup,
					})
					if err != nil {
						return err
					}

					return provisioner.Run(ctx)
				},
			},
			{
				Name: "vault-unseal",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "address",
						Sources: cli.EnvVars("ADDRESS"),
					},
					&cli.BoolFlag{
						Name:    "run-forever",
						Value:   true,
						Sources: cli.EnvVars("RUN_FOREVER"),
					},
					&cli.StringFlag{
						Name:     "unseal-key-path",
						Required: true,
						Sources:  cli.EnvVars("UNSEAL_KEY_PATH"),
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					address := c.String("address")
					runForever := c.Bool("run-forever")
					unsealKeyPath := c.String("unseal-key-path")

					unsealer, err := unseal.New(&unseal.Opts{
						Address:       address,
						RunForever:    ptr.Get(runForever),
						UnsealKeyPath: unsealKeyPath,
					})
					if err != nil {
						return err
					}
					return unsealer.Run(ctx)
				},
			},
			{
				Name: "vault-push-secrets",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "address",
						Value:   "http://localhost:8200",
						Sources: cli.EnvVars("ADDRESS"),
					},
					&cli.StringFlag{
						Name:    "role",
						Sources: cli.EnvVars("ROLE"),
					},
					&cli.BoolFlag{
						Name:    "run-forever",
						Value:   true,
						Sources: cli.EnvVars("RUN_FOREVER"),
					},
					&cli.StringFlag{
						Name:     "secrets-path",
						Required: true,
						Sources:  cli.EnvVars("SECRETS_PATH"),
					},
					&cli.StringFlag{
						Name:     "storage-path",
						Required: true,
						Sources:  cli.EnvVars("STORAGE_PATH"),
					},
					&cli.StringFlag{
						Name:     "storage-credentials-path",
						Required: true,
						Sources:  cli.EnvVars("STORAGE_CREDENTIALS_PATH"),
					},
					&cli.StringFlag{
						Name:    "token",
						Sources: cli.EnvVars("TOKEN"),
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					address := c.String("address")
					role := c.String("role")
					runForever := c.Bool("run-forever")
					secretsPath := c.String("secrets-path")
					storagePath := c.String("storage-path")
					storageCredentialsPath := c.String("storage-credentials-path")
					token := c.String("token")

					pusher, err := push.New(&push.Opts{
						Address:                address,
						Role:                   role,
						RunForever:             ptr.Get(runForever),
						SecretsPath:            secretsPath,
						StoragePath:            storagePath,
						StorageCredentialsPath: storageCredentialsPath,
						Token:                  token,
					})
					if err != nil {
						return err
					}

					return pusher.Run(ctx)
				},
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-format",
				Sources: cli.EnvVars("LOG_FORMAT"),
				Value:   "text",
			},
			&cli.StringFlag{
				Name:    "log-level",
				Sources: cli.EnvVars("LOG_LEVEL"),
				Value:   "info",
			},
		},
		Version: info.Version,
	}

	err := command.Run(context.Background(), os.Args)
	code := 0
	if err != nil {
		fmt.Printf("command failed, error: %v", err)
		code = 1
	}
	os.Exit(code)
}
