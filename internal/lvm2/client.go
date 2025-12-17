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

type ThinLV struct {
	LogicalVolume string
	Pool          string
	Size          string
	VolumeGroup   string
}

type ThinLVPool struct {
	ChunkSize     string
	LogicalVolume string
	Size          string
	VolumeGroup   string
	Zero          *bool
}

func (c *Client) CreateLV(ctx context.Context, lv any) error {
	command := []string{"lvcreate"}

	if tplv, ok := lv.(ThinLVPool); ok {
		if tplv.LogicalVolume == "" {
			return fmt.Errorf("thin pool logical volume unset")
		}
		if tplv.VolumeGroup == "" {
			return fmt.Errorf("thin pool volume group unset")
		}

		extents := ""
		if tplv.Size == "" {
			extents = "100%FREE"
		}

		var zeroStr string
		if tplv.Zero != nil {
			if *tplv.Zero {
				zeroStr = "y"
			} else {
				zeroStr = "n"
			}
		}

		command = append(command, "--type", "thin-pool")
		if tplv.Size != "" {
			command = append(command, "--size", tplv.Size)
		}
		if extents != "" {
			command = append(command, "--extents", extents)
		}
		if tplv.ChunkSize != "" {
			command = append(command, "--chunksize", tplv.ChunkSize)
		}
		if zeroStr != "" {
			command = append(command, "--zero", zeroStr)
		}
		command = append(command, "--name", tplv.LogicalVolume)
		command = append(command, tplv.VolumeGroup)
	} else if tlv, ok := lv.(ThinLV); ok {
		if tlv.LogicalVolume == "" {
			return fmt.Errorf("thin logical volume unset")
		}
		if tlv.Pool == "" {
			return fmt.Errorf("thin pool unset")
		}
		if tlv.Size == "" {
			return fmt.Errorf("thin size unset")
		}
		if tlv.VolumeGroup == "" {
			return fmt.Errorf("thin volume group unset")
		}

		command = append(command, "--type", "thin")
		command = append(command, "--virtualsize", tlv.Size)
		command = append(command, "--thinpool", tlv.Pool)
		command = append(command, "--name", tlv.LogicalVolume)
		command = append(command, tlv.VolumeGroup)
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

func (c *Client) ExtendLV(ctx context.Context, vg string, lv string, size string) error {
	if size == "" {
		size = "100%FREE"
	}

	groupAndVolume := fmt.Sprintf("%s/%s", vg, lv)
	_, err := process.Output(ctx, []string{"lvextend", "--extents", size, groupAndVolume})
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
