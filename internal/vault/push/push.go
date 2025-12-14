package push

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/goccy/go-yaml"
	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	"google.golang.org/api/option"
)

type Opts struct {
	Address                string
	Interval               time.Duration
	RunForever             bool
	SecretsPath            string
	StoragePath            string
	StorageCredentialsPath string
	Token                  string
}

type Pusher struct {
	Address      string
	Interval     time.Duration
	LastChecksum string
	RunForever   bool
	SecretsPath  string
	Storage      *storage.Client
	StoragePath  string
	Token        string
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

	_, _, err = ParseStoragePath(opts.StoragePath)
	if err != nil {
		return nil, err
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
		Token:       opts.Token,
		Vault:       vaultClient,
	}
	return &pusher, nil
}

var storagePathRegex = regexp.MustCompile("^gs://([^/]+)/(.*[^/])$")

func ParseStoragePath(storagePath string) (string, string, error) {
	matches := storagePathRegex.FindStringSubmatch(storagePath)
	if matches == nil {
		return "", "", fmt.Errorf("invalid storage path %s", storagePath)
	}
	bucket := matches[1]
	path := matches[2]
	return bucket, path, nil
}

func (p *Pusher) ExportSecrets(ctx context.Context, secretsPath string) (map[string]any, error) {
	response, err := p.Vault.Secrets.KvV2List(ctx, "", vault.WithMountPath(secretsPath))
	if err != nil {
		return nil, err
	}
	apps := response.Data.Keys

	data := map[string]any{}
	for _, app := range apps {
		response, err := p.Vault.Secrets.KvV2Read(ctx, app, vault.WithMountPath(secretsPath))
		if err != nil {
			return nil, err
		}
		data[app] = response.Data.Data
	}

	return data, nil
}

func (p *Pusher) Checksum(ctx context.Context, data map[string]any) (string, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(dataBytes)

	return fmt.Sprintf("%x", hash), nil
}

func (p *Pusher) Upload(ctx context.Context, storagePath string, data map[string]any) error {
	bucket, path, err := ParseStoragePath(storagePath)
	if err != nil {
		return err
	}

	dataBytes, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	reader := bytes.NewReader(dataBytes)
	writer := p.Storage.Bucket(bucket).Object(path).NewWriter(ctx)
	defer writer.Close()

	_, err = io.Copy(writer, reader)
	if err != nil {
		return err
	}

	return nil
}

func (p *Pusher) AuthVault(ctx context.Context, addresss string) error {
	token := p.Token
	if token == "" {
		jwtBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
		if err != nil {
			return err
		}
		jwt := string(jwtBytes)

		response, err := p.Vault.Auth.KubernetesLogin(ctx, schema.KubernetesLoginRequest{
			Jwt:  jwt,
			Role: "vault-push-secrets",
		})
		if err != nil {
			return err
		}

		token = response.Auth.ClientToken
	}

	err := p.Vault.SetToken(token)
	if err != nil {
		return err
	}

	return nil
}

func (p *Pusher) Push(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("pushing vault secrets to cloud storage")

	logger.Info("logging into vault", "address", p.Address)
	err := p.AuthVault(ctx, p.Address)
	if err != nil {
		return err
	}
	defer p.Vault.ClearToken()

	logger.Info("exporting secrets", "address", p.Address)
	secrets, err := p.ExportSecrets(ctx, p.SecretsPath)
	if err != nil {
		return err
	}

	logger.Info("calculating checksum")
	checksum, err := p.Checksum(ctx, secrets)
	if err != nil {
		return err
	}
	if checksum == p.LastChecksum {
		logger.Info("secrets are unchanged")
		return nil
	}
	p.LastChecksum = checksum

	logger.Info("uploading secrets", "storage-path", p.StoragePath)
	err = p.Upload(ctx, p.StoragePath, secrets)
	if err != nil {
		return err
	}

	return nil
}

func (p *Pusher) Run(ctx context.Context) error {
	if !p.RunForever {
		return p.Push(ctx)
	}

	logger := logging.FromContext(ctx)
	logger.Info("starting loop", "interval", p.Interval)

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
