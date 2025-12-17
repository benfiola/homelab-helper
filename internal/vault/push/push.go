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
	Role                   string
	RunForever             *bool
	SecretsPath            string
	StoragePath            string
	StorageCredentialsPath string
	Token                  string
}

type Pusher struct {
	Address      string
	Interval     time.Duration
	LastChecksum string
	Role         string
	RunForever   bool
	SecretsPath  string
	Storage      *storage.Client
	StoragePath  string
	Token        string
	Vault        *vault.Client
}

func New(opts *Opts) (*Pusher, error) {
	if opts.Address == "" {
		return nil, fmt.Errorf("address unset")
	}

	interval := opts.Interval
	if interval == 0 {
		interval = 10 * time.Minute
	}

	if opts.Role == "" {
		return nil, fmt.Errorf("role unset")
	}

	runForever := true
	if opts.RunForever != nil {
		runForever = *opts.RunForever
	}

	if opts.SecretsPath == "" {
		return nil, fmt.Errorf("secrets path unset")
	}

	if opts.StorageCredentialsPath == "" {
		return nil, fmt.Errorf("storage credentials path unset")
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
		Interval:    interval,
		Role:        opts.Role,
		RunForever:  runForever,
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

func (p *Pusher) ExportSecrets(ctx context.Context) (map[string]any, error) {
	logger := logging.FromContext(ctx)

	response, err := p.Vault.Secrets.KvV2List(ctx, "", vault.WithMountPath(p.SecretsPath))
	if err != nil {
		logger.Error("failed to list secrets from vault", "secrets-path", p.SecretsPath, "error", err)
		return nil, err
	}
	apps := response.Data.Keys

	data := map[string]any{}
	for _, app := range apps {
		response, err := p.Vault.Secrets.KvV2Read(ctx, app, vault.WithMountPath(p.SecretsPath))
		if err != nil {
			logger.Error("failed to read secret", "app", app, "error", err)
			return nil, err
		}
		data[app] = response.Data.Data
	}

	return data, nil
}

func (p *Pusher) Checksum(ctx context.Context, data map[string]any) (string, error) {
	logger := logging.FromContext(ctx)

	dataBytes, err := json.Marshal(data)
	if err != nil {
		logger.Error("failed to marshal secrets for checksum calculation", "error", err)
		return "", err
	}

	hash := sha256.Sum256(dataBytes)
	checksum := fmt.Sprintf("%x", hash)

	return checksum, nil
}

func (p *Pusher) Upload(ctx context.Context, data map[string]any) error {
	logger := logging.FromContext(ctx)

	bucket, path, err := ParseStoragePath(p.StoragePath)
	if err != nil {
		logger.Error("failed to parse storage path", "storage-path", p.StoragePath, "error", err)
		return err
	}

	dataBytes, err := yaml.Marshal(data)
	if err != nil {
		logger.Error("failed to marshal secrets to YAML", "error", err)
		return err
	}

	reader := bytes.NewReader(dataBytes)
	writer := p.Storage.Bucket(bucket).Object(path).NewWriter(ctx)
	defer writer.Close()

	_, err = io.Copy(writer, reader)
	if err != nil {
		logger.Error("failed to upload to cloud storage", "bucket", bucket, "path", path, "error", err)
		return err
	}

	return nil
}

func (p *Pusher) AuthVault(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	token := p.Token
	if token == "" {
		tokenPath := "/var/run/secrets/kubernetes.io/serviceaccount/token"
		jwtBytes, err := os.ReadFile(tokenPath)
		if err != nil {
			logger.Error("failed to read service account token", "path", tokenPath, "error", err)
			return err
		}
		jwt := string(jwtBytes)

		response, err := p.Vault.Auth.KubernetesLogin(ctx, schema.KubernetesLoginRequest{
			Jwt:  jwt,
			Role: p.Role,
		})
		if err != nil {
			logger.Error("failed to authenticate with vault using kubernetes", "role", p.Role, "error", err)
			return err
		}

		token = response.Auth.ClientToken
	}

	err := p.Vault.SetToken(token)
	if err != nil {
		logger.Error("failed to set vault client token", "error", err)
		return err
	}

	return nil
}

func (p *Pusher) Push(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	logger.Debug("authenticating with vault")
	err := p.AuthVault(ctx)
	if err != nil {
		logger.Error("vault authentication failed", "error", err)
		return err
	}
	defer p.Vault.ClearToken()

	logger.Debug("exporting secrets")
	secrets, err := p.ExportSecrets(ctx)
	if err != nil {
		logger.Error("failed to export secrets", "error", err)
		return err
	}

	logger.Debug("calculating checksum")
	checksum, err := p.Checksum(ctx, secrets)
	if err != nil {
		logger.Error("failed to calculate checksum", "error", err)
		return err
	}

	if checksum == p.LastChecksum {
		logger.Info("secrets unchanged, skipping upload")
		return nil
	}
	logger.Debug("secrets changed, uploading", "previous-checksum", p.LastChecksum, "current-checksum", checksum)
	p.LastChecksum = checksum

	err = p.Upload(ctx, secrets)
	if err != nil {
		logger.Error("failed to upload secrets", "error", err)
		return err
	}

	logger.Info("secrets successfully pushed", "checksum", checksum)
	return nil
}

func (p *Pusher) Run(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("starting vault push", "vault", p.Address)

	err := p.Push(ctx)
	if err != nil {
		logger.Error("initial push failed", "error", err)
		return err
	}

	if !p.RunForever {
		return nil
	}

	logger.Info("entering continuous push loop", "interval", p.Interval)

	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

	running := true
	pushCount := 0
	for running {
		select {
		case <-ticker.C:
			pushCount++
			logger.Debug("executing scheduled push", "push-number", pushCount)
			err := p.Push(ctx)
			if err != nil {
				logger.Error("push failed", "push-number", pushCount, "error", err)
				return err
			}
		case sig := <-signalChannel:
			logger.Info("shutdown signal received", "signal", sig)
			running = false
		}
	}

	logger.Info("vault push shutdown complete")
	return nil
}
