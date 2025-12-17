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
	logger.Debug("resolving partition label symlink", "symlink", symlink)

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

	logger.Info("resolved partition label to device", "partition-label", p.PartitionLabel, "device", absPath)
	return absPath, nil
}

func (p *DiskProvisioner) GetSatelliteID(ctx context.Context) (string, error) {
	logger := logging.FromContext(ctx)
	device := fmt.Sprintf("/dev/%s/%s", p.VolumeGroup, p.MetadataLV)

	_, err := os.Lstat(device)
	if err != nil {
		logger.Debug("metadata device does not exist, satellite id unavailable", "device", device)
		return "", nil
	}

	logger.Debug("mounting metadata device to read satellite id", "device", device)
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
		logger.Debug("failed to read satellite-id file", "file", file, "error", err)
		return "", nil
	}

	data := string(dataBytes)
	data = strings.TrimSpace(data)
	logger.Info("retrieved satellite id from metadata", "satellite-id", data)
	return data, nil
}

func (p *DiskProvisioner) GroupAndVolume(vg string, lv string) string {
	return fmt.Sprintf("%s/%s", vg, lv)
}

func (p *DiskProvisioner) ListPVs(ctx context.Context) ([]string, error) {
	logger := logging.FromContext(ctx)
	logger.Debug("querying physical volumes")

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

	logger.Debug("found physical volumes", "count", len(pvs), "pvs", pvs)
	return pvs, nil
}

func (p *DiskProvisioner) ListVGs(ctx context.Context) ([]string, error) {
	logger := logging.FromContext(ctx)
	logger.Debug("querying volume groups")

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

	logger.Debug("found volume groups", "count", len(vgs), "vgs", vgs)
	return vgs, nil
}

func (p *DiskProvisioner) ListLVs(ctx context.Context) ([]string, error) {
	logger := logging.FromContext(ctx)
	logger.Debug("querying logical volumes")

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

	logger.Debug("found logical volumes", "count", len(lvs), "lvs", lvs)
	return lvs, nil
}

func (p *DiskProvisioner) CreateMetadataLV(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("creating metadata logical volume", "lv-name", p.MetadataLV, "size", "100M", "pool", p.Pool)

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
	logger.Debug("formatting metadata device with ext4", "device", device)

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

	logger.Debug("mounting metadata device", "device", device, "mount-point", mount)
	_, err = process.Output(ctx, []string{"mount", device, mount})
	if err != nil {
		logger.Error("failed to mount metadata device", "device", device, "error", err)
		return err
	}
	defer func() {
		process.Output(ctx, []string{"umount", mount})
	}()

	file := fmt.Sprintf("%s/satellite-id", mount)
	logger.Debug("writing satellite id to metadata file", "file", file, "satellite-id", p.SatelliteID)

	err = os.WriteFile(file, []byte(p.SatelliteID), 0644)
	if err != nil {
		logger.Error("failed to write satellite-id file", "file", file, "error", err)
		return err
	}

	logger.Info("successfully created and initialized metadata logical volume")
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

	if !slices.Contains(lvs, p.GroupAndVolume(p.VolumeGroup, p.Pool)) {
		logger.Info("creating thin pool", "logical-volume", p.Pool)
		err = p.Client.CreateLV(ctx, lvm2.ThinLVPool{
			ChunkSize:     "512K",
			LogicalVolume: p.Pool,
			VolumeGroup:   p.VolumeGroup,
			Zero:          ptr.Get(false),
		})
		if err != nil {
			return err
		}
	}

	logger.Info("extending thin pool", "logical-volume", p.Pool)
	p.Client.ExtendLV(ctx, p.VolumeGroup, p.Pool, "")

	if !slices.Contains(lvs, p.GroupAndVolume(p.VolumeGroup, p.MetadataLV)) {
		logger.Info("creating metadata logical volume", "logical-volume", p.MetadataLV)
		err = p.CreateMetadataLV(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *DiskProvisioner) Run(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	logger.Info("starting disk provisioning process")

	err := p.Provision(ctx)
	if err != nil {
		logger.Error("disk provisioning failed", "error", err)
		return err
	}

	logger.Info("disk provisioning completed successfully")
	return nil
}
