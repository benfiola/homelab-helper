package push

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/goccy/go-yaml"
	"github.com/hashicorp/vault-client-go"
	"google.golang.org/api/option"
)

type Opts struct {
	Address                string
	Interval               time.Duration
	RunForever             bool
	SecretsPath            string
	StoragePath            string
	StorageCredentialsPath string
}

type Pusher struct {
	Address      string
	Interval     time.Duration
	LastChecksum string
	RunForever   bool
	SecretsPath  string
	Storage      *storage.Client
	StoragePath  string
	Vault        *vault.Client
}

func New(opts *Opts) (*Pusher, error) {
	interval := opts.Interval
	if interval == 0 {
		interval = 10 * time.Minute
	}

	if opts.SecretsPath == "" {
		return nil, fmt.Errorf("secrets path unset")
	}

	storageClient, err := storage.NewClient(context.Background(), option.WithCredentialsFile(opts.StorageCredentialsPath))
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(opts.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("invalid storage path %s", opts.StoragePath)
	}
	if parsed.Scheme != "gs" && parsed.Path == "" || parsed.Path == "/" {
		return nil, fmt.Errorf("invalid storage path %s", opts.StoragePath)
	}

	vaultClient, err := vault.New(
		vault.WithAddress(opts.Address),
	)
	if err != nil {
		return nil, err
	}

	pusher := Pusher{
		Address:     opts.Address,
		Interval:    opts.Interval,
		RunForever:  opts.RunForever,
		SecretsPath: opts.SecretsPath,
		Storage:     storageClient,
		StoragePath: opts.StoragePath,
		Vault:       vaultClient,
	}
	return &pusher, nil
}

func (p *Pusher) Push(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("pushing vault secrets to cloud storage")

	logger.Info("listing kv paths", "address", p.Address)
	response, err := p.Vault.Secrets.KvV2List(ctx, p.SecretsPath)
	if err != nil {
		return err
	}
	apps := response.Data.Keys

	secrets := map[string]map[string]any{}
	for _, path := range apps {
		logger.Info("get kv path", "address", p.Address, "path", path)
		fullPath := fmt.Sprintf("%s/%s", p.SecretsPath, path)
		response, err := p.Vault.Secrets.KvV2Read(ctx, fullPath)
		if err != nil {
			return err
		}
		secret := response.Data.Data
		secrets[path] = secret
	}

	parsed, err := url.Parse(p.StoragePath)
	if err != nil {
		return err
	}
	storageBucket := parsed.Hostname()
	storagePath := parsed.Path

	logger.Info("uploading to cloud storage", "bucket", storageBucket, "file", storagePath)

	secretsBytes, err := yaml.Marshal(secrets)
	if err != nil {
		return err
	}

	secretsReader := bytes.NewReader(secretsBytes)
	bucketWriter := p.Storage.Bucket(storageBucket).Object(storagePath).NewWriter(ctx)
	_, err = io.Copy(bucketWriter, secretsReader)
	if err != nil {
		return err
	}

	err = bucketWriter.Close()
	if err != nil {
		return err
	}

	return nil
}

func (p *Pusher) Run(ctx context.Context) error {
	if !p.RunForever {
		return p.Push(ctx)
	}

	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

	running := false
	for running {
		select {
		case <-ticker.C:
			err := p.Push(ctx)
			if err != nil {
				return err
			}
		case <-signalChannel:
			running = false
		}
	}

	return nil
}
