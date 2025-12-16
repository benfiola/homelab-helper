package diskprovisioner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/benfiola/homelab-helper/internal/logging"
	"github.com/benfiola/homelab-helper/internal/lvm2"
	"github.com/benfiola/homelab-helper/internal/process"
	"github.com/benfiola/homelab-helper/internal/ptr"
)

type Opts struct {
	PartitionLabel string
	Pool           string
	SatelliteID    string
	VolumeGroup    string
}

type DiskProvisioner struct {
	Client         *lvm2.Client
	MetadataLV     string
	PartitionLabel string
	Pool           string
	SatelliteID    string
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

	if opts.SatelliteID == "" {
		return nil, fmt.Errorf("satellite id unset")
	}

	if opts.VolumeGroup == "" {
		return nil, fmt.Errorf("volume group unset")
	}

	provisioner := DiskProvisioner{
		Client:         client,
		MetadataLV:     "metadata",
		PartitionLabel: opts.PartitionLabel,
		Pool:           opts.Pool,
		SatelliteID:    opts.SatelliteID,
		VolumeGroup:    opts.VolumeGroup,
	}
	return &provisioner, nil
}

func (p *DiskProvisioner) ResolvePartitionLabel(ctx context.Context) (string, error) {
	symlink := fmt.Sprintf("/dev/disk/by-partlabel/%s", p.PartitionLabel)
	relPath, err := os.Readlink(symlink)
	if err != nil {
		return "", err
	}

	absPath := filepath.Join(filepath.Dir(symlink), relPath)
	if absPath == symlink {
		return "", fmt.Errorf("could not resolve device symlink '%s'", symlink)
	}
	return absPath, nil
}

func (p *DiskProvisioner) GetSatelliteID(ctx context.Context) (string, error) {
	device := fmt.Sprintf("/dev/%s/%s", p.VolumeGroup, p.MetadataLV)
	_, err := os.Lstat(device)
	if err != nil {
		return "", nil
	}

	mount, err := os.MkdirTemp("", "")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(mount)

	_, err = process.Output(ctx, []string{"mount", device, mount})
	if err != nil {
		return "", err
	}
	defer func() {
		process.Output(ctx, []string{"unmount", mount})
	}()

	file := fmt.Sprintf("%s/satellite-id", mount)
	dataBytes, err := os.ReadFile(file)
	if err != nil {
		return "", nil
	}

	data := string(dataBytes)
	data = strings.TrimSpace(data)
	return data, nil
}

func (p *DiskProvisioner) ListPVs(ctx context.Context) ([]string, error) {
	data, err := p.Client.ShowPV(ctx)
	if err != nil {
		return nil, err
	}

	pvMap := map[string]bool{}
	for _, item := range data.Report {
		for _, currPv := range item.PV {
			pvMap[currPv.PVName] = true
		}
	}

	pvs := []string{}
	for pv := range pvMap {
		pvs = append(pvs, pv)
	}

	return pvs, nil
}

func (p *DiskProvisioner) ListVGs(ctx context.Context) ([]string, error) {
	data, err := p.Client.ShowVG(ctx)
	if err != nil {
		return nil, err
	}

	vgMap := map[string]bool{}
	for _, item := range data.Report {
		for _, currVg := range item.VG {
			vgMap[currVg.VGName] = true
		}
	}

	vgs := []string{}
	for vg := range vgMap {
		vgs = append(vgs, vg)
	}

	return vgs, nil
}

func (p *DiskProvisioner) ListLVs(ctx context.Context) ([]string, error) {
	data, err := p.Client.ShowLV(ctx)
	if err != nil {
		return nil, err
	}

	lvMap := map[string]bool{}
	for _, item := range data.Report {
		for _, currLv := range item.LV {
			lvMap[currLv.LVName] = true
		}
	}

	lvs := []string{}
	for lv := range lvMap {
		lvs = append(lvs, lv)
	}

	return lvs, nil
}

func (p *DiskProvisioner) CreateMetadataLV(ctx context.Context) error {
	pool := fmt.Sprintf("/dev/%s/%s", p.VolumeGroup, p.Pool)
	lv := fmt.Sprintf("/dev/%s/%s", p.VolumeGroup, p.MetadataLV)

	err := p.Client.CreateLV(ctx, lvm2.ThinLV{
		LV:   lv,
		Pool: pool,
	})
	if err != nil {
		return err
	}

	_, err = process.Output(ctx, []string{"mkfs.ext4", lv})
	if err != nil {
		return err
	}

	mount, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(mount)

	_, err = process.Output(ctx, []string{"mount", lv, mount})
	if err != nil {
		return err
	}
	defer func() {
		process.Output(ctx, []string{"unmount", mount})
	}()

	file := fmt.Sprintf("%s/satellite-id", mount)

	err = os.WriteFile(file, []byte(p.SatelliteID), 0644)
	if err != nil {
		return err
	}

	return nil
}

func (p *DiskProvisioner) Provision(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("provisioning disks")

	logger.Info("resolving symlink", "partition-label", p.PartitionLabel)
	pv, err := p.ResolvePartitionLabel(ctx)
	if err != nil {
		return err
	}

	logger.Info("listing all pvs")
	pvs, err := p.ListPVs(ctx)
	if err != nil {
		return err
	}

	logger.Info("listing all vgs")
	vgs, err := p.ListVGs(ctx)
	if err != nil {
		return err
	}

	logger.Info("getting known satellite id")
	satelliteID, err := p.GetSatelliteID(ctx)
	if err != nil {
		return err
	}

	if p.SatelliteID != satelliteID {
		logger.Info("satellite id differs", "known", satelliteID, "expected", p.SatelliteID)

		for _, vg := range vgs {
			logger.Info("removing all logical volumes for volume group", "volume-group", vg)
			err := p.Client.RemoveAllLVs(ctx, vg)
			if err != nil {
				return err
			}

			logger.Info("removing volume group", "volume-group", vg)
			err = p.Client.RemoveVG(ctx, vg)
			if err != nil {
				return err
			}
		}

		for _, pv := range pvs {
			logger.Info("removing pv", "physical-volume", pv)
			err = p.Client.RemovePV(ctx, pv)
			if err != nil {
				return err
			}
		}

		logger.Info("(re-)listing all pvs")
		pvs, err = p.ListPVs(ctx)
		if err != nil {
			return err
		}

		logger.Info("(re-)listing all vgs")
		vgs, err = p.ListVGs(ctx)
		if err != nil {
			return err
		}
	}

	if !slices.Contains(pvs, pv) {
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

	if !slices.Contains(vgs, p.VolumeGroup) {
		logger.Info("creating volume group", "physical-volume", pv, "volume-group", p.VolumeGroup)
		err = p.Client.CreateVG(ctx, p.VolumeGroup, pv)
		if err != nil {
			return err
		}
	}

	lvs, err := p.ListLVs(ctx)
	if err != nil {
		return err
	}

	thinpool := fmt.Sprintf("%s/%s", p.VolumeGroup, p.Pool)
	if !slices.Contains(lvs, thinpool) {
		logger.Info("creating thin pool", "logical-volume", thinpool)
		err = p.Client.CreateLV(ctx, lvm2.ThinLVPool{
			ChunkSize: "512K",
			LV:        thinpool,
			Zero:      ptr.Get(false),
		})
		if err != nil {
			return err
		}
	}

	logger.Info("extending thin pool", "logical-volume", thinpool)
	p.Client.ExtendLV(ctx, thinpool, "")

	metadata := fmt.Sprintf("%s/%s", p.VolumeGroup, p.MetadataLV)
	if !slices.Contains(lvs, metadata) {
		logger.Info("creating metadata logical volume", "logical-volume", "metadata")
		err = p.CreateMetadataLV(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *DiskProvisioner) Run(ctx context.Context) error {
	err := p.Provision(ctx)
	if err != nil {
		return err
	}

	return nil
}
