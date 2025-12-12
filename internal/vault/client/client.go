package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/benfiola/homelab-helper/internal/process"
)

type Opts struct {
	Address string
	Token   string
}

type Client struct {
	Address string
	Token   string
}

func New(opts *Opts) (*Client, error) {
	address := opts.Address
	if address == "" {
		address = "http://localhost:8200"
	}

	client := Client{
		Address: address,
		Token:   opts.Token,
	}
	return &client, nil
}

type AuthEngine struct{}

func (c *Client) ListAuthEngines(ctx context.Context) (map[string]AuthEngine, error) {
	output, err := process.Output(ctx, []string{"vault", "auth", "list", "--format=json"})
	if err != nil {
		return nil, err
	}

	data := map[string]AuthEngine{}
	err = json.Unmarshal([]byte(output), &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (c *Client) EnableAuthEngine(ctx context.Context, engine string) error {
	_, err := process.Output(ctx, []string{"vault", "auth", "enable", engine})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) ListPolicies(ctx context.Context) ([]string, error) {
	output, err := process.Output(ctx, []string{"vault", "policy", "list", "--format=json"})
	if err != nil {
		return nil, err
	}

	data := []string{}
	err = json.Unmarshal([]byte(output), &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (c *Client) CreatePolicy(ctx context.Context, name string, content string) error {
	file, err := os.CreateTemp("", "")
	if err != nil {
		return err
	}
	defer file.Close()
	defer os.Remove(file.Name())

	_, err = file.WriteString(content)
	if err != nil {
		return err
	}

	_, err = process.Output(ctx, []string{"vault", "policy", "write", name, file.Name()})
	if err != nil {
		return err
	}

	return nil
}

type SecretEngine struct{}

func (c *Client) ListSecretEngines(ctx context.Context) (map[string]SecretEngine, error) {
	output, err := process.Output(ctx, []string{"vault", "secrets", "list", "--format=json"})
	if err != nil {
		return nil, err
	}

	data := map[string]SecretEngine{}
	err = json.Unmarshal([]byte(output), &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type KV2 struct {
	Description string
	Path        string
	Version     string
}

func (c *Client) EnableSecretEngine(ctx context.Context, engine any) error {
	command := []string{"vault", "secrets", "enable"}

	if kv2, ok := engine.(KV2); ok {
		version := kv2.Version
		if version == "" {
			version = "2"
		}

		command = append(command, fmt.Sprintf("--path=%s", kv2.Path))
		command = append(command, fmt.Sprintf("--description=%s", kv2.Description))
		command = append(command, fmt.Sprintf("--version=%s", version))
		command = append(command, "kv2")
	} else {
		return fmt.Errorf("unimplemented")
	}

	_, err := process.Output(ctx, command)
	if err != nil {
		return err
	}

	return nil
}

type Status struct {
	Initialized bool `json:"initialized"`
	Sealed      bool `json:"sealed"`
}

func (c *Client) Status(ctx context.Context) (*Status, error) {
	output, err := process.Output(ctx, []string{"vault", "status", fmt.Sprintf("-address=%s", c.Address), "-format=json"})
	if eerr, ok := err.(*exec.ExitError); ok {
		if eerr.ExitCode() == 2 {
			err = nil
		}
	}
	if err != nil {
		return nil, err
	}

	status := Status{}
	err = json.Unmarshal([]byte(output), &status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

func (c *Client) Unseal(ctx context.Context, key string) error {
	_, err := process.Output(ctx, []string{"vault", "operator", "unseal", fmt.Sprintf("-address=%s", c.Address), key})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) Read(ctx context.Context, path string) (string, error) {
	output, err := process.Output(ctx, []string{"vault", "read", path, "--format=json"})
	if err != nil {
		return "", err
	}

	return output, err
}
func (c *Client) Write(ctx context.Context, path string, data map[string]string) error {
	command := []string{"vault", "write", path}
	for k, v := range data {
		command = append(command, fmt.Sprintf("%s=%s", k, v))
	}

	_, err := process.Output(ctx, command)
	if err != nil {
		return err
	}

	return nil
}
