package lvm2

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/benfiola/homelab-helper/internal/process"
)

type Opts struct {
}

type Client struct {
}

func New(opts *Opts) (*Client, error) {
	client := Client{}
	return &client, nil
}

func (c *Client) CreatePV(ctx context.Context, device string) error {
	_, err := process.Output(ctx, "pvcreate", device)
	if err != nil {
		return err
	}

	return nil
}

type PVInfo struct {
}

func (c *Client) DisplayPV(ctx context.Context, device string) (*PVInfo, error) {
	output, err := process.Output(ctx, "pvdisplay", "--reportformat=json", device)
	if err != nil {
		return nil, err
	}

	pvinfo := PVInfo{}
	err = json.Unmarshal([]byte(output), &pvinfo)
	if err != nil {
		return nil, err
	}

	return &pvinfo, nil
}

func (c *Client) ResizePV(ctx context.Context, device string) error {
	_, err := process.Output(ctx, "pvresize", device)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) CreateVG(ctx context.Context, name string, device string) error {
	_, err := process.Output(ctx, "vgcreate", name, device)
	if err != nil {
		return err
	}

	return nil
}

type VGInfo struct {
}

func (c *Client) DisplayVG(ctx context.Context, name string) (*VGInfo, error) {
	output, err := process.Output(ctx, "vgdisplay", "--reportformat=json", name)
	if err != nil {
		return nil, err
	}

	vginfo := VGInfo{}
	err = json.Unmarshal([]byte(output), &vginfo)
	if err != nil {
		return nil, err
	}

	return &vginfo, nil
}

type ThinLV struct {
	ChunkSize string
	LV        string
	Size      string
	Zero      *bool
}

type Options struct {
	ThinLV *ThinLV
}

type Option func(*Options) error

func WithThinLV(tlv *ThinLV) Option {
	return func(o *Options) error {
		o.ThinLV = tlv
		return nil
	}
}

func (c *Client) CreateLV(ctx context.Context, name string, opts ...Option) error {
	options := Options{}
	for _, opt := range opts {
		err := opt(&options)
		if err != nil {
			return err
		}
	}

	command := []string{"lvcreate"}

	if options.ThinLV != nil {
		t := options.ThinLV

		size := t.Size
		if size == "" {
			size = "100%FREE"
		}

		var zeroStr string
		if t.Zero != nil {
			if *t.Zero {
				zeroStr = "y"
			} else {
				zeroStr = "n"
			}
		}

		command = append(command, "--extents", size, "--thin", t.LV)
		if t.ChunkSize != "" {
			command = append(command, "--chunksize", t.ChunkSize)
		}
		if zeroStr != "" {
			command = append(command, "--zero", zeroStr)
		}
	} else {
		return fmt.Errorf("unimplemented")
	}

	_, err := process.Output(ctx, command...)
	if err != nil {
		return err
	}

	return nil
}

type LVInfo struct {
}

func (c *Client) DisplayLV(ctx context.Context, name string) (*LVInfo, error) {
	output, err := process.Output(ctx, "lvdisplay", "--reportformat=json", name)
	if err != nil {
		return nil, err
	}

	lvinfo := LVInfo{}
	err = json.Unmarshal([]byte(output), &lvinfo)
	if err != nil {
		return nil, err
	}

	return &lvinfo, nil
}

func (c *Client) ExtendLV(ctx context.Context, volume string, size string) error {
	if size == "" {
		size = "100%FREE"
	}

	_, err := process.Output(ctx, "lvextend", "--extents", size, volume)
	if err != nil {
		return err
	}

	return nil
}
