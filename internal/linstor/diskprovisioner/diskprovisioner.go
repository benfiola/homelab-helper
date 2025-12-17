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
	logger := logging.FromContext(ctx)
	symlink := fmt.Sprintf("/dev/disk/by-partlabel/%s", p.PartitionLabel)

	relPath, err := os.Readlink(symlink)
	if err != nil {
		logger.Error("failed to read symlink", "symlink", symlink, "error", err)
		return "", err
	}

	absPath := filepath.Join(filepath.Dir(symlink), relPath)
	if absPath == symlink {
		logger.Error("symlink resolution failed - circular reference", "symlink", symlink)
		return "", fmt.Errorf("could not resolve device symlink '%s'", symlink)
	}

	return absPath, nil
}

func (p *DiskProvisioner) GetSatelliteID(ctx context.Context) (string, error) {
	logger := logging.FromContext(ctx)
	device := fmt.Sprintf("/dev/%s/%s", p.VolumeGroup, p.MetadataLV)

	_, err := os.Lstat(device)
	if err != nil {
		return "", nil
	}

	mount, err := os.MkdirTemp("", "")
	if err != nil {
		logger.Error("failed to create temporary mount directory", "error", err)
		return "", err
	}
	defer os.RemoveAll(mount)

	_, err = process.Output(ctx, []string{"mount", device, mount})
	if err != nil {
		logger.Error("failed to mount metadata device", "device", device, "mount-point", mount, "error", err)
		return "", err
	}
	defer func() {
		process.Output(ctx, []string{"umount", mount})
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

func (p *DiskProvisioner) GroupAndVolume(vg string, lv string) string {
	return fmt.Sprintf("%s/%s", vg, lv)
}

func (p *DiskProvisioner) ListPVs(ctx context.Context) ([]string, error) {
	logger := logging.FromContext(ctx)

	data, err := p.Client.ShowPV(ctx)
	if err != nil {
		logger.Error("failed to query physical volumes", "error", err)
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
	logger := logging.FromContext(ctx)

	data, err := p.Client.ShowVG(ctx)
	if err != nil {
		logger.Error("failed to query volume groups", "error", err)
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
	logger := logging.FromContext(ctx)

	data, err := p.Client.ShowLV(ctx)
	if err != nil {
		logger.Error("failed to query logical volumes", "error", err)
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
	logger := logging.FromContext(ctx)

	err := p.Client.CreateLV(ctx, lvm2.ThinLV{
		LogicalVolume: p.MetadataLV,
		Pool:          p.Pool,
		Size:          "100M",
		VolumeGroup:   p.VolumeGroup,
	})
	if err != nil {
		logger.Error("failed to create metadata logical volume", "error", err)
		return err
	}

	device := fmt.Sprintf("/dev/%s/%s", p.VolumeGroup, p.MetadataLV)

	_, err = process.Output(ctx, []string{"mkfs.ext4", device})
	if err != nil {
		logger.Error("failed to format metadata device", "device", device, "error", err)
		return err
	}

	mount, err := os.MkdirTemp("", "")
	if err != nil {
		logger.Error("failed to create temporary mount directory", "error", err)
		return err
	}
	defer os.RemoveAll(mount)

	_, err = process.Output(ctx, []string{"mount", device, mount})
	if err != nil {
		logger.Error("failed to mount metadata device", "device", device, "error", err)
		return err
	}
	defer func() {
		process.Output(ctx, []string{"umount", mount})
	}()

	file := fmt.Sprintf("%s/satellite-id", mount)

	err = os.WriteFile(file, []byte(p.SatelliteID), 0644)
	if err != nil {
		logger.Error("failed to write satellite-id file", "file", file, "error", err)
		return err
	}

	return nil
}

func (p *DiskProvisioner) Provision(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	logger.Debug("resolving partition label", "partition-label", p.PartitionLabel)
	pv, err := p.ResolvePartitionLabel(ctx)
	if err != nil {
		logger.Error("failed to resolve partition label", "error", err)
		return err
	}

	logger.Debug("listing physical volumes")
	pvs, err := p.ListPVs(ctx)
	if err != nil {
		logger.Error("failed to list physical volumes", "error", err)
		return err
	}

	logger.Debug("listing volume groups")
	vgs, err := p.ListVGs(ctx)
	if err != nil {
		logger.Error("failed to list volume groups", "error", err)
		return err
	}

	logger.Debug("retrieving satellite id")
	satelliteID, err := p.GetSatelliteID(ctx)
	if err != nil {
		logger.Error("failed to retrieve satellite id", "error", err)
		return err
	}

	if p.SatelliteID != satelliteID {
		logger.Info("satellite id mismatch, resetting lvm configuration", "existing", satelliteID, "expected", p.SatelliteID)

		for _, vg := range vgs {
			logger.Debug("removing logical volumes", "volume-group", vg)
			err := p.Client.RemoveAllLVs(ctx, vg)
			if err != nil {
				logger.Error("failed to remove logical volumes", "volume-group", vg, "error", err)
				return err
			}

			logger.Debug("removing volume group", "volume-group", vg)
			err = p.Client.RemoveVG(ctx, vg)
			if err != nil {
				logger.Error("failed to remove volume group", "volume-group", vg, "error", err)
				return err
			}
		}

		for _, pv := range pvs {
			logger.Debug("removing physical volume", "physical-volume", pv)
			err = p.Client.RemovePV(ctx, pv)
			if err != nil {
				logger.Error("failed to remove physical volume", "physical-volume", pv, "error", err)
				return err
			}
		}

		logger.Debug("re-listing physical volumes")
		pvs, err = p.ListPVs(ctx)
		if err != nil {
			logger.Error("failed to re-list physical volumes", "error", err)
			return err
		}

		logger.Debug("re-listing volume groups")
		vgs, err = p.ListVGs(ctx)
		if err != nil {
			logger.Error("failed to re-list volume groups", "error", err)
			return err
		}
	}

	if !slices.Contains(pvs, pv) {
		logger.Debug("creating physical volume", "physical-volume", pv)
		err = p.Client.CreatePV(ctx, pv)
		if err != nil {
			logger.Error("failed to create physical volume", "physical-volume", pv, "error", err)
			return err
		}
	}

	logger.Debug("resizing physical volume", "physical-volume", pv)
	err = p.Client.ResizePV(ctx, pv)
	if err != nil {
		logger.Error("failed to resize physical volume", "physical-volume", pv, "error", err)
		return err
	}

	if !slices.Contains(vgs, p.VolumeGroup) {
		logger.Debug("creating volume group", "physical-volume", pv, "volume-group", p.VolumeGroup)
		err = p.Client.CreateVG(ctx, p.VolumeGroup, pv)
		if err != nil {
			logger.Error("failed to create volume group", "volume-group", p.VolumeGroup, "error", err)
			return err
		}
	}

	lvs, err := p.ListLVs(ctx)
	if err != nil {
		logger.Error("failed to list logical volumes", "error", err)
		return err
	}

	if !slices.Contains(lvs, p.GroupAndVolume(p.VolumeGroup, p.Pool)) {
		logger.Debug("creating thin pool", "pool", p.Pool, "volume-group", p.VolumeGroup)
		err = p.Client.CreateLV(ctx, lvm2.ThinLVPool{
			ChunkSize:     "512K",
			LogicalVolume: p.Pool,
			VolumeGroup:   p.VolumeGroup,
			Zero:          ptr.Get(false),
		})
		if err != nil {
			logger.Error("failed to create thin pool", "pool", p.Pool, "error", err)
			return err
		}
	}

	logger.Debug("extending thin pool", "pool", p.Pool, "volume-group", p.VolumeGroup)
	p.Client.ExtendLV(ctx, p.VolumeGroup, p.Pool, "")

	if !slices.Contains(lvs, p.GroupAndVolume(p.VolumeGroup, p.MetadataLV)) {
		logger.Debug("creating metadata logical volume", "lv", p.MetadataLV)
		err = p.CreateMetadataLV(ctx)
		if err != nil {
			logger.Error("failed to create metadata logical volume", "error", err)
			return err
		}
	}

	logger.Info("disk provisioning completed successfully")
	return nil
}

func (p *DiskProvisioner) Run(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("starting disk provisioning")

	err := p.Provision(ctx)
	if err != nil {
		logger.Error("disk provisioning failed", "error", err)
		return err
	}

	return nil
}
