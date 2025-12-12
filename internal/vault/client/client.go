package client

import (
	"context"
	"encoding/json"
	"fmt"
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
