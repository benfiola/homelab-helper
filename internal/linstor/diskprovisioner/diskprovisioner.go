package diskprovisioner

import (
	"context"
	"fmt"
	"os"

	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/benfiola/homelab-helper/internal/lvm2"
)

type Opts struct {
	PartitionLabel string
	Pool           string
	VolumeGroup    string
}

type DiskProvisioner struct {
	PartitionLabel string
	Pool           string
	VolumeGroup    string
}

func New(opts *Opts) (*DiskProvisioner, error) {
	if opts.PartitionLabel == "" {
		return nil, fmt.Errorf("partition label unset")
	}
	if opts.Pool == "" {
		return nil, fmt.Errorf("pool unset")
	}
	if opts.VolumeGroup == "" {
		return nil, fmt.Errorf("volume group unset")
	}

	provisioner := DiskProvisioner{
		PartitionLabel: opts.PartitionLabel,
		Pool:           opts.Pool,
		VolumeGroup:    opts.VolumeGroup,
	}
	return &provisioner, nil
}

func (p *DiskProvisioner) Provision(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("provisioning disks")

	lvm, err := lvm2.New(&lvm2.Opts{})
	if err != nil {
		return err
	}

	logger.Info("resolving symlink", "partition-label", p.PartitionLabel)
	symlink := fmt.Sprintf("/dev/disk/by-partlabel/%s", p.PartitionLabel)
	pv, err := os.Readlink(symlink)
	if err != nil {
		return err
	}
	if pv == symlink {
		return fmt.Errorf("could not resolve device symlink '%s", symlink)
	}

	_, err = lvm.DisplayPV(ctx, pv)
	if err != nil {
		logger.Info("creating physical volume", "physical-volume", pv)
		err = lvm.CreatePV(ctx, pv)
		if err != nil {
			return err
		}
	}

	logger.Info("resizing physical volume", "physical-volume", pv)
	err = lvm.ResizePV(ctx, pv)
	if err != nil {
		return err
	}

	_, err = lvm.DisplayVG(ctx, p.VolumeGroup)
	if err != nil {
		logger.Info("resizing volume group", "volume-group", p.VolumeGroup)
		err = lvm.CreateVG(ctx, p.VolumeGroup, pv)
		if err != nil {
			return err
		}
	}

	lv := fmt.Sprintf("%s/%s", p.VolumeGroup, p.Pool)
	_, err = lvm.DisplayLV(ctx, lv)
	if err != nil {
		logger.Info("creating logical volume", "logical-volume", lv)
		err = lvm.CreateLV(ctx, lv, lvm2.WithThinLV(&lvm2.ThinLV{
			ChunkSize: "512K",
			LV:        lv,
			Zero:      false,
		}))
		if err != nil {
			return err
		}
	}

	logger.Info("extending logical volume", "logical-volume", lv)
	lvm.ExtendLV(ctx, lv, "")

	return nil
}
