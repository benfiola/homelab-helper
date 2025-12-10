package diskprovisioner

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/benfiola/homelab-helper/internal/lvm2"
	"github.com/benfiola/homelab-helper/internal/ptr"
)

type Opts struct {
	PartitionLabel string
	Pool           string
	RunForever     *bool
	VolumeGroup    string
}

type DiskProvisioner struct {
	Client         *lvm2.Client
	PartitionLabel string
	Pool           string
	RunForever     bool
	VolumeGroup    string
}

func New(opts *Opts) (*DiskProvisioner, error) {
	client, err := lvm2.New(&lvm2.Opts{})
	if err != nil {
		return nil, err
	}

	if opts.PartitionLabel == "" {
		return nil, fmt.Errorf("partition label unset")
	}

	if opts.Pool == "" {
		return nil, fmt.Errorf("pool unset")
	}

	runForever := true
	if opts.RunForever != nil {
		runForever = *opts.RunForever
	}

	if opts.VolumeGroup == "" {
		return nil, fmt.Errorf("volume group unset")
	}

	provisioner := DiskProvisioner{
		Client:         client,
		PartitionLabel: opts.PartitionLabel,
		Pool:           opts.Pool,
		RunForever:     runForever,
		VolumeGroup:    opts.VolumeGroup,
	}
	return &provisioner, nil
}

func (p *DiskProvisioner) Provision(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("provisioning disks")

	logger.Info("resolving symlink", "partition-label", p.PartitionLabel)
	symlink := fmt.Sprintf("/dev/disk/by-partlabel/%s", p.PartitionLabel)
	pv, err := os.Readlink(symlink)
	if err != nil {
		return err
	}
	pv = filepath.Join(filepath.Dir(symlink), pv)
	if pv == symlink {
		return fmt.Errorf("could not resolve device symlink '%s", symlink)
	}

	_, err = p.Client.DisplayPV(ctx, pv)
	if err != nil {
		logger.Info("creating physical volume", "physical-volume", pv)
		err = p.Client.CreatePV(ctx, pv)
		if err != nil {
			return err
		}
	}

	logger.Info("resizing physical volume", "physical-volume", pv)
	err = p.Client.ResizePV(ctx, pv)
	if err != nil {
		return err
	}

	_, err = p.Client.DisplayVG(ctx, p.VolumeGroup)
	if err != nil {
		logger.Info("creating volume group", "physical-volume", pv, "volume-group", p.VolumeGroup)
		err = p.Client.CreateVG(ctx, p.VolumeGroup, pv)
		if err != nil {
			return err
		}
	}

	lv := fmt.Sprintf("%s/%s", p.VolumeGroup, p.Pool)
	_, err = p.Client.DisplayLV(ctx, lv)
	if err != nil {
		logger.Info("creating logical volume", "logical-volume", lv)
		err = p.Client.CreateLV(ctx, lv, lvm2.WithThinLV(&lvm2.ThinLV{
			ChunkSize: "512K",
			LV:        lv,
			Zero:      ptr.Get(false),
		}))
		if err != nil {
			return err
		}
	}

	logger.Info("extending logical volume", "logical-volume", lv)
	p.Client.ExtendLV(ctx, lv, "")

	if p.RunForever {
		signalChannel := make(chan os.Signal, 1)
		signal.Notify(signalChannel, syscall.SIGTERM, syscall.SIGINT)
		<-signalChannel
	}

	return nil
}
