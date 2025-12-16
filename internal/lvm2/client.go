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
	_, err := process.Output(ctx, []string{"pvcreate", device})
	if err != nil {
		return err
	}

	return nil
}

type PVInfo struct {
	Report []struct {
		PV []struct {
			PVName string `json:"pv_name"`
		} `json:"pv"`
	} `json:"report"`
}

func (c *Client) ShowPV(ctx context.Context) (*PVInfo, error) {
	output, err := process.Output(ctx, []string{"pvs", "--reportformat=json"})
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
	_, err := process.Output(ctx, []string{"pvresize", device})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RemovePV(ctx context.Context, device string) error {
	_, err := process.Output(ctx, []string{"pvremove", "-f", device})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) CreateVG(ctx context.Context, name string, device string) error {
	_, err := process.Output(ctx, []string{"vgcreate", name, device})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RemoveVG(ctx context.Context, name string) error {
	_, err := process.Output(ctx, []string{"vgremove", "-f", name})
	if err != nil {
		return err
	}

	return nil
}

type VGInfo struct {
	Report []struct {
		VG []struct {
			VGName string `json:"vg_name"`
		} `json:"vg"`
	} `json:"report"`
}

func (c *Client) ShowVG(ctx context.Context) (*VGInfo, error) {
	output, err := process.Output(ctx, []string{"vgs", "--reportformat=json"})
	if err != nil {
		return nil, err
	}

	vgInfo := VGInfo{}
	err = json.Unmarshal([]byte(output), &vgInfo)
	if err != nil {
		return nil, err
	}

	return &vgInfo, nil
}

type LV struct {
	LV   string
	Pool string
	Size string
}

type ThinLVPool struct {
	ChunkSize string
	LV        string
	Size      string
	Zero      *bool
}

func (c *Client) CreateLV(ctx context.Context, lv any) error {
	command := []string{"lvcreate"}

	if tplv, ok := lv.(ThinLVPool); ok {
		size := tplv.Size
		if size == "" {
			size = "100%FREE"
		}

		var zeroStr string
		if tplv.Zero != nil {
			if *tplv.Zero {
				zeroStr = "y"
			} else {
				zeroStr = "n"
			}
		}

		command = append(command, "--extents", size, "--thinpool", tplv.LV)
		if tplv.ChunkSize != "" {
			command = append(command, "--chunksize", tplv.ChunkSize)
		}
		if zeroStr != "" {
			command = append(command, "--zero", zeroStr)
		}
	} else if tlv, ok := lv.(ThinLV); ok {
		
	} else {
		return fmt.Errorf("unimplemented")
	}

	_, err := process.Output(ctx, command)
	if err != nil {
		return err
	}

	return nil
}

type LVInfo struct {
	Report []struct {
		LV []struct {
			LVName string `json:"lv_name"`
		} `json:"lv"`
	} `json:"report"`
}

func (c *Client) ShowLV(ctx context.Context) (*LVInfo, error) {
	output, err := process.Output(ctx, []string{"lvs", "--reportformat=json"})
	if err != nil {
		return nil, err
	}

	lvInfo := LVInfo{}
	err = json.Unmarshal([]byte(output), &lvInfo)
	if err != nil {
		return nil, err
	}

	return &lvInfo, nil
}

func (c *Client) ExtendLV(ctx context.Context, volume string, size string) error {
	if size == "" {
		size = "100%FREE"
	}

	_, err := process.Output(ctx, []string{"lvextend", "--extents", size, volume})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RemoveAllLVs(ctx context.Context, vg string) error {
	_, err := process.Output(ctx, []string{"lvremove", "-f", vg})
	if err != nil {
		return err
	}

	return nil
}
