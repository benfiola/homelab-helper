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

func (p *DiskProvisioner) ResolvePartitionLabel(ctx context.Context, label string) (string, error) {
	symlink := fmt.Sprintf("/dev/disk/by-partlabel/%s", p.PartitionLabel)
	relPath, err := os.Readlink(symlink)
	if err != nil {
		return "", err
	}
	absPath := filepath.Join(filepath.Dir(symlink), relPath)
	if absPath == symlink {
		return "", fmt.Errorf("could not resolve device symlink '%s", symlink)
	}
	return absPath, nil
}

func (p *DiskProvisioner) ExistsPV(ctx context.Context, pv string) (bool, error) {
	data, err := p.Client.ShowPV(ctx)
	if err != nil {
		return false, err
	}

	found := false
	for _, item := range data.Report {
		for _, currPv := range item.PV {
			if currPv.PVName == pv {
				found = true
				break
			}
		}
	}

	return found, nil
}

func (p *DiskProvisioner) ExistsVG(ctx context.Context, vg string) (bool, error) {
	data, err := p.Client.ShowVG(ctx)
	if err != nil {
		return false, err
	}

	found := false
	for _, item := range data.Report {
		for _, currVg := range item.VG {
			if currVg.VGName == vg {
				found = true
				break
			}
		}
	}

	return found, nil
}

func (p *DiskProvisioner) ExistsLV(ctx context.Context, lv string) (bool, error) {
	data, err := p.Client.ShowLV(ctx)
	if err != nil {
		return false, err
	}

	found := false
	for _, item := range data.Report {
		for _, currLv := range item.LV {
			if currLv.LVName == lv {
				found = true
				break
			}
		}
	}

	return found, nil
}

func (p *DiskProvisioner) Provision(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("provisioning disks")

	logger.Info("resolving symlink", "partition-label", p.PartitionLabel)
	pv, err := p.ResolvePartitionLabel(ctx, p.PartitionLabel)
	if err != nil {
		return err
	}

	existsPV, err := p.ExistsPV(ctx, pv)
	if err != nil {
		return err
	}

	if !existsPV {
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

	existsVG, err := p.ExistsVG(ctx, p.VolumeGroup)
	if err != nil {
		return err
	}

	if !existsVG {
		logger.Info("creating volume group", "physical-volume", pv, "volume-group", p.VolumeGroup)
		err = p.Client.CreateVG(ctx, p.VolumeGroup, pv)
		if err != nil {
			return err
		}
	}

	existsLV, err := p.ExistsLV(ctx, p.Pool)
	if err != nil {
		return err
	}

	lv := fmt.Sprintf("%s/%s", p.VolumeGroup, p.Pool)
	if !existsLV {
		logger.Info("creating logical volume", "logical-volume", lv)
		err = p.Client.CreateLV(ctx, lvm2.ThinLV{
			ChunkSize: "512K",
			LV:        lv,
			Zero:      ptr.Get(false),
		})
		if err != nil {
			return err
		}
	}

	logger.Info("extending logical volume", "logical-volume", lv)
	p.Client.ExtendLV(ctx, lv, "")

	return nil
}

func (p *DiskProvisioner) Run(ctx context.Context) error {

	err := p.Provision(ctx)
	if err != nil {
		return err
	}

	if p.RunForever {
		logger := logging.FromContext(ctx)
		logger.Info("sleeping until signal")
		signalChannel := make(chan os.Signal, 1)
		signal.Notify(signalChannel, syscall.SIGTERM, syscall.SIGINT)
		<-signalChannel
	}

	return nil
}
