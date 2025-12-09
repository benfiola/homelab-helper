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
}

type Client struct {
	Address string
}

func New(opts *Opts) (*Client, error) {
	address := opts.Address
	if address == "" {
		address = "http://localhost:8200"
	}

	client := Client{
		Address: address,
	}
	return &client, nil
}

type Status struct {
	Initialized bool
	Unsealed    bool
}

func (c *Client) Status(ctx context.Context) (*Status, error) {
	output, err := process.Output(ctx, "vault", "status", fmt.Sprintf("--address=%s", c.Address), "--format=json")
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
	_, err := process.Output(ctx, "vault", "operator", "unseal", fmt.Sprintf("--address=%s", c.Address), key)
	if err != nil {
		return err
	}

	return nil
}
