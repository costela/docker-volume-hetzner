package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/pkg/mount"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/pkg/errors"
)

// used in methods that take &bools
var trueVar = true
var falseVar = false

type hetznerDriver struct {
	client hetznerClienter
}

func newHetznerDriver() *hetznerDriver {
	return &hetznerDriver{
		client: &hetznerClient{hcloud.NewClient(hcloud.WithToken(strings.TrimSpace(os.Getenv("apikey"))))},
	}
}

func (hd *hetznerDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{
		Capabilities: volume.Capability{Scope: "global"},
	}
}

func (hd *hetznerDriver) Create(req *volume.CreateRequest) error {
	validateOptions(req.Name, req.Options)

	prefixedName := prefixName(req.Name)

	logrus.Infof("starting volume creation for '%s'", prefixedName)

	size, err := strconv.Atoi(getOption("size", req.Options))
	if err != nil {
		return errors.Wrapf(err, "could not convert size '%s' to int", getOption("size", req.Options))
	}

	srv, err := hd.getServerForLocalhost()
	if err != nil {
		return err
	}

	opts := hcloud.VolumeCreateOpts{
		Name:     prefixedName,
		Size:     size,
		Location: srv.Datacenter.Location, // attach explicitly to be able to wait
		Labels:   map[string]string{"docker-volume-hetzner": ""},
	}

	resp, _, err := hd.client.Volume().Create(context.Background(), opts)
	if err != nil {
		return errors.Wrapf(err, "could not create volume '%s'", prefixedName)
	}
	if err := hd.waitForAction(resp.Action); err != nil {
		return errors.Wrapf(err, "could not create volume '%s'", prefixedName)
	}

	logrus.Infof("volume '%s' (%dGB) created on '%s'; attaching", prefixedName, size, srv.Name)

	act, _, err := hd.client.Volume().Attach(context.Background(), resp.Volume, srv)
	if err != nil {
		return errors.Wrapf(err, "could not attach volume '%s' to '%s'", prefixedName, srv.Name)
	}
	if err := hd.waitForAction(act); err != nil {
		return errors.Wrapf(err, "could not attach volume '%s' to '%s'", prefixedName, srv.Name)
	}

	logrus.Infof("volume '%s' attached to '%s'", prefixedName, srv.Name)

	if useProtection() {
		// be optimistic for now and ignore errors here
		_, _, _ = hd.client.Volume().ChangeProtection(context.Background(), resp.Volume, hcloud.VolumeChangeProtectionOpts{Delete: &trueVar})
	}

	logrus.Infof("formatting '%s' as '%s'", prefixedName, getOption("fstype", req.Options))
	err = mkfs(resp.Volume.LinuxDevice, getOption("fstype", req.Options))
	if err != nil {
		return errors.Wrapf(err, "could not mkfs on '%s'", resp.Volume.LinuxDevice)
	}

	return nil
}

func (hd *hetznerDriver) List() (*volume.ListResponse, error) {
	logrus.Infof("got list request")

	vols, err := hd.client.Volume().All(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "could not list all volumes")
	}

	mounts, err := getMounts()
	if err != nil {
		return nil, errors.Wrap(err, "could not get local mounts")
	}

	resp := volume.ListResponse{
		Volumes: make([]*volume.Volume, 0, len(vols)),
	}
	for _, vol := range vols {
		if !nameHasPrefix(vol.Name) {
			continue
		}
		v := &volume.Volume{
			Name: unprefixedName(vol.Name),
		}
		if mountpoint, ok := mounts[vol.LinuxDevice]; ok {
			v.Mountpoint = mountpoint
		}
		resp.Volumes = append(resp.Volumes, v)
	}

	return &resp, nil
}

func (hd *hetznerDriver) Get(req *volume.GetRequest) (*volume.GetResponse, error) {
	prefixedName := prefixName(req.Name)

	logrus.Infof("fetching information for volume '%s'", prefixedName)

	vol, _, err := hd.client.Volume().GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return nil, errors.Wrapf(err, "could not get cloud volume '%s'", prefixedName)
	}

	mounts, err := getMounts()
	if err != nil {
		return nil, errors.Wrap(err, "could not get local mounts")
	}

	status := make(map[string]interface{})

	mountpoint, mounted := mounts[vol.LinuxDevice]
	if mounted {
		status["mounted"] = true
	}

	resp := volume.GetResponse{
		Volume: &volume.Volume{
			Name:       unprefixedName(vol.Name),
			Mountpoint: mountpoint,
			CreatedAt:  vol.Created.Format(time.RFC3339),
			Status:     status,
		},
	}

	logrus.Infof("returning info on '%s': %#v", prefixedName, resp.Volume)

	return &resp, nil
}

func (hd *hetznerDriver) Remove(req *volume.RemoveRequest) error {
	prefixedName := prefixName(req.Name)

	logrus.Infof("starting volume removal for '%s'", prefixedName)

	vol, _, err := hd.client.Volume().GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return errors.Wrapf(err, "could not get cloud volume '%s'", prefixedName)
	}

	if useProtection() {
		logrus.Infof("disabling protection for '%s'", prefixedName)
		act, _, err := hd.client.Volume().ChangeProtection(context.Background(), vol, hcloud.VolumeChangeProtectionOpts{Delete: &falseVar})
		if err != nil {
			return errors.Wrapf(err, "could not unprotect volume '%s'", prefixedName)
		}
		if err := hd.waitForAction(act); err != nil {
			return errors.Wrapf(err, "could not unprotect volume '%s'", prefixedName)
		}
	}

	if vol.Server != nil && vol.Server.ID != 0 {
		logrus.Infof("detaching volume '%s' (attached to %d)", prefixedName, vol.Server.ID)
		act, _, err := hd.client.Volume().Detach(context.Background(), vol)
		if err != nil {
			return errors.Wrapf(err, "could not detach volume '%s'", prefixedName)
		}
		if err := hd.waitForAction(act); err != nil {
			return errors.Wrapf(err, "could not detach volume '%s'", prefixedName)
		}
	}

	_, err = hd.client.Volume().Delete(context.Background(), vol)
	if err != nil {
		return errors.Wrapf(err, "could not delete volume '%s'", prefixedName)
	}

	logrus.Infof("volume '%s' removed successfully", prefixedName)

	return nil
}

func (hd *hetznerDriver) Path(req *volume.PathRequest) (*volume.PathResponse, error) {
	prefixedName := prefixName(req.Name)

	logrus.Infof("got path request for volume '%s'", prefixedName)

	resp, err := hd.Get(&volume.GetRequest{Name: req.Name})
	if err != nil {
		return nil, errors.Wrapf(err, "could not get path for volume '%s'", prefixedName)
	}

	return &volume.PathResponse{Mountpoint: resp.Volume.Mountpoint}, nil
}

func (hd *hetznerDriver) Mount(req *volume.MountRequest) (*volume.MountResponse, error) {
	prefixedName := prefixName(req.Name)

	logrus.Infof("received mount request for '%s' as '%s'", prefixedName, req.ID)

	vol, _, err := hd.client.Volume().GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return nil, errors.Wrapf(err, "could not get volume '%s'", prefixedName)
	}

	if vol.Server != nil && vol.Server.ID != 0 {
		volSrv, _, err := hd.client.Server().GetByID(context.Background(), vol.Server.ID)
		if err != nil {
			return nil, errors.Wrapf(err, "could not fetch server details for volume '%s'", prefixedName)
		}
		vol.Server = volSrv
	}

	srv, err := hd.getServerForLocalhost()
	if err != nil {
		return nil, err
	}

	if vol.Server == nil || vol.Server.Name != srv.Name {
		if vol.Server != nil && vol.Server.Name != "" {
			logrus.Infof("detaching volume '%s' from '%s'", prefixedName, vol.Server.Name)
			act, _, err := hd.client.Volume().Detach(context.Background(), vol)
			if err != nil {
				return nil, errors.Wrapf(err, "could not detach volume '%s' from '%s'", vol.Name, vol.Server.Name)
			}
			if err := hd.waitForAction(act); err != nil {
				return nil, errors.Wrapf(err, "could not detach volume '%s' from '%s'", vol.Name, vol.Server.Name)
			}
		}
		logrus.Infof("attaching volume '%s' to '%s'", prefixedName, srv.Name)
		act, _, err := hd.client.Volume().Attach(context.Background(), vol, srv)
		if err != nil {
			return nil, errors.Wrapf(err, "could not attach volume '%s' to '%s'", vol.Name, srv.Name)
		}
		if err := hd.waitForAction(act); err != nil {
			return nil, errors.Wrapf(err, "could not attach volume '%s' to '%s'", vol.Name, srv.Name)
		}
	}

	mountpoint := fmt.Sprintf("%s/%s", propagatedMountPath, req.ID)

	logrus.Infof("creating mountpoint %s", mountpoint)
	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		return nil, errors.Wrapf(err, "could not create mountpoint %s", mountpoint)
	}

	logrus.Infof("mounting '%s' on '%s'", prefixedName, mountpoint)

	// copy busybox' approach and just try everything we expect might work
	var merr error
	mounted := false
	for _, fstype := range supportedFileystemTypes {
		if err := mount.Mount(vol.LinuxDevice, mountpoint, fstype, ""); err == nil {
			mounted = true
			break
		}
		merr = multierror.Append(merr, err)
	}
	if !mounted {
		return nil, errors.Wrapf(merr, "could not mount '%s' as any of %s", vol.LinuxDevice, supportedFileystemTypes)
	}

	logrus.Infof("successfully mounted '%s' on '%s'", prefixedName, mountpoint)

	return &volume.MountResponse{Mountpoint: mountpoint}, nil
}

func (hd *hetznerDriver) Unmount(req *volume.UnmountRequest) error {
	prefixedName := prefixName(req.Name)

	logrus.Infof("received unmount request for '%s' as '%s'", prefixedName, req.ID)

	vol, _, err := hd.client.Volume().GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return errors.Wrapf(err, "could not get volume '%s'", prefixedName)
	}

	mountpoint := fmt.Sprintf("%s/%s", propagatedMountPath, req.ID)

	if err := mount.Unmount(mountpoint); err != nil {
		return errors.Wrapf(err, "could not unmount '%s'", mountpoint)
	}

	logrus.Infof("unmounted '%s'", mountpoint)

	if err := os.Remove(mountpoint); err != nil {
		return errors.Wrapf(err, "could not remove mountpoint %s", mountpoint)
	}

	srv, err := hd.getServerForLocalhost()
	if err != nil {
		return nil
	}

	if vol.Server == nil || vol.Server.Name != srv.Name {
		return nil
	}

	logrus.Infof("detaching volume '%s'", prefixedName)

	act, _, err := hd.client.Volume().Detach(context.Background(), vol)
	if err != nil {
		return errors.Wrapf(err, "could not detach volume '%s'", vol.Name)
	}
	if err := hd.waitForAction(act); err != nil {
		return errors.Wrapf(err, "could not detach volume '%s'", vol.Name)
	}

	return nil
}

func (hd *hetznerDriver) getServerForLocalhost() (*hcloud.Server, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, errors.Wrap(err, "could not get local hostname")
	}

	if strings.Contains(hostname, ".") {
		logrus.Warnf("hostname contains dot ('%s'); make sure hostname != FQDN and matches the hcloud server name", hostname)
	}

	srv, _, err := hd.client.Server().GetByName(context.Background(), hostname)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get cloud server '%s'", hostname)
	}

	return srv, nil
}

func (hd *hetznerDriver) waitForAction(act *hcloud.Action) error {
	_, errs := hd.client.Action().WatchProgress(context.Background(), act)
	return <-errs
}

func validateOptions(volume string, opts map[string]string) {
	for k := range opts {
		switch k {
		case "fstype", "size": // OK, noop
		default:
			logrus.Warnf("unsupported driver_opt '%s' for volume %s", k, volume)
		}
	}
}

func getOption(k string, opts map[string]string) string {
	if v, ok := opts[k]; ok {
		return v
	}
	return os.Getenv(k)
}

func prefixName(name string) string {
	s := fmt.Sprintf("%s-%s", os.Getenv("prefix"), name)
	if len(s) > 64 {
		return s[:64]
	}
	return s
}

func unprefixedName(name string) string {
	return strings.TrimPrefix(name, fmt.Sprintf("%s-", os.Getenv("prefix")))
}

func nameHasPrefix(name string) bool {
	return strings.HasPrefix(name, fmt.Sprintf("%s-", os.Getenv("prefix")))
}

func useProtection() bool {
	return os.Getenv("use_protection") == "true"
}
