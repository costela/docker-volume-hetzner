package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/pkg/mount"
	log "github.com/sirupsen/logrus"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/pkg/errors"
)

// used in methods that take &bools
var trueVar = true
var falseVar = false

type hetznerDriver struct {
	client *hcloud.Client
}

func newHetznerDriver() *hetznerDriver {
	return &hetznerDriver{
		client: hcloud.NewClient(hcloud.WithToken(strings.TrimSpace(os.Getenv("apikey")))),
	}
}

func (hd *hetznerDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{
		Capabilities: volume.Capability{Scope: "global"},
	}
}

func (hd *hetznerDriver) Create(req *volume.CreateRequest) error {
	prefixedName := prefixName(req.Name)

	log.Infof("starting volume creation for '%s'", prefixedName)

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

	resp, _, err := hd.client.Volume.Create(context.Background(), opts)
	if err != nil {
		return errors.Wrapf(err, "could not create volume '%s'", prefixedName)
	}
	if err := hd.waitForAction(resp.Action); err != nil {
		return errors.Wrapf(err, "could not create volume '%s'", prefixedName)
	}

	log.Infof("volume '%s' (%dGB) created on '%s'; attaching", prefixedName, size, srv.Name)

	act, _, err := hd.client.Volume.Attach(context.Background(), resp.Volume, srv)
	if err != nil {
		return errors.Wrapf(err, "could not attach volume '%s' to '%s'", prefixedName, srv.Name)
	}
	if err := hd.waitForAction(act); err != nil {
		return errors.Wrapf(err, "could not attach volume '%s' to '%s'", prefixedName, srv.Name)
	}

	log.Infof("volume '%s' attached to '%s'", prefixedName, srv.Name)

	// be optimistic for now and ignore errors here
	hd.client.Volume.ChangeProtection(context.Background(), resp.Volume, hcloud.VolumeChangeProtectionOpts{Delete: &trueVar})

	log.Infof("formatting '%s' as '%s'", prefixedName, getOption("fstype", req.Options))
	err = mkfs(resp.Volume.LinuxDevice, getOption("fstype", req.Options))
	if err != nil {
		return errors.Wrapf(err, "could not mkfs on '%s'", resp.Volume.LinuxDevice)
	}

	return nil
}

func (hd *hetznerDriver) List() (*volume.ListResponse, error) {
	log.Infof("got list request")

	vols, err := hd.client.Volume.All(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "could not list all volumes")
	}

	mounts, err := getMounts()
	if err != nil {
		return nil, errors.Wrap(err, "could not get local mounts")
	}

	resp := volume.ListResponse{
		Volumes: make([]*volume.Volume, len(vols)),
	}
	for i, vol := range vols {
		resp.Volumes[i] = &volume.Volume{
			Name: unprefixedName(vol.Name),
		}
		if mountpoint, ok := mounts[vol.LinuxDevice]; ok {
			resp.Volumes[i].Mountpoint = mountpoint
		}
	}

	return &resp, nil
}

func (hd *hetznerDriver) Get(req *volume.GetRequest) (*volume.GetResponse, error) {
	prefixedName := prefixName(req.Name)

	log.Infof("fetching information for volume '%s'", prefixedName)

	vol, _, err := hd.client.Volume.GetByName(context.Background(), prefixedName)
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
		// v2 plugins (container-based) should answer with a path inside PropagatedMount
		mountpoint = strings.TrimPrefix(mountpoint, propagatedMountPath)
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

	log.Infof("returning info on '%s': %#v", prefixedName, resp.Volume)

	return &resp, nil
}

func (hd *hetznerDriver) Remove(req *volume.RemoveRequest) error {
	prefixedName := prefixName(req.Name)

	log.Infof("starting volume removal for '%s'", prefixedName)

	vol, _, err := hd.client.Volume.GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return errors.Wrapf(err, "could not get cloud volume '%s'", prefixedName)
	}

	log.Infof("disabling protection for '%s'", prefixedName)
	act, _, err := hd.client.Volume.ChangeProtection(context.Background(), vol, hcloud.VolumeChangeProtectionOpts{Delete: &falseVar})
	if err != nil {
		return errors.Wrapf(err, "could not unprotect volume '%s'", prefixedName)
	}
	if err := hd.waitForAction(act); err != nil {
		return errors.Wrapf(err, "could not unprotect volume '%s'", prefixedName)
	}

	if vol.Server != nil && vol.Server.ID != 0 {
		log.Infof("detaching volume '%s' (attached to %d)", prefixedName, vol.Server.ID)
		act, _, err = hd.client.Volume.Detach(context.Background(), vol)
		if err != nil {
			return errors.Wrapf(err, "could not detach volume '%s'", prefixedName)
		}
		if err := hd.waitForAction(act); err != nil {
			return errors.Wrapf(err, "could not detach volume '%s'", prefixedName)
		}
	}

	_, err = hd.client.Volume.Delete(context.Background(), vol)
	if err != nil {
		return errors.Wrapf(err, "could not delete volume '%s'", prefixedName)
	}

	log.Infof("volume '%s' removed successfully", prefixedName)

	return nil
}

func (hd *hetznerDriver) Path(req *volume.PathRequest) (*volume.PathResponse, error) {
	prefixedName := prefixName(req.Name)

	log.Infof("got path request for volume '%s'", prefixedName)

	resp, err := hd.Get(&volume.GetRequest{Name: req.Name})
	if err != nil {
		return nil, errors.Wrapf(err, "could not get path for volume '%s'", prefixedName)
	}

	return &volume.PathResponse{Mountpoint: resp.Volume.Mountpoint}, nil
}

func (hd *hetznerDriver) Mount(req *volume.MountRequest) (*volume.MountResponse, error) {
	prefixedName := prefixName(req.Name)

	log.Infof("received mount request for '%s' as '%s'", prefixedName, req.ID)

	vol, _, err := hd.client.Volume.GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return nil, errors.Wrapf(err, "could not get volume '%s'", prefixedName)
	}

	if vol.Server != nil && vol.Server.ID != 0 {
		volSrv, _, err := hd.client.Server.GetByID(context.Background(), vol.Server.ID)
		if err != nil {
			return nil, errors.Wrapf(err, "could not fetch server details for volume '%s'", prefixedName)
		}
		log.Debugf("fetched server info for volume '%s': %#v", prefixedName, volSrv)
		vol.Server = volSrv
	}

	srv, err := hd.getServerForLocalhost()
	if err != nil {
		return nil, err
	}

	if vol.Server.Name != srv.Name {
		log.Infof("maybe detaching volume '%s' from '%s'?", prefixedName, vol.Server.Name)
		if vol.Server.Name != "" {

			act, _, err := hd.client.Volume.Detach(context.Background(), vol)
			if err != nil {
				return nil, errors.Wrapf(err, "could not detach volume '%s' from '%s'", vol.Name, vol.Server.Name)
			}
			if err := hd.waitForAction(act); err != nil {
				return nil, errors.Wrapf(err, "could not detach volume '%s' from '%s'", vol.Name, vol.Server.Name)
			}
		}
		log.Infof("attaching volume '%s' to '%s'", prefixedName, srv.Name)
		act, _, err := hd.client.Volume.Attach(context.Background(), vol, srv)
		if err != nil {
			return nil, errors.Wrapf(err, "could not attach volume '%s' to '%s'", vol.Name, srv.Name)
		}
		if err := hd.waitForAction(act); err != nil {
			return nil, errors.Wrapf(err, "could not attach volume '%s' to '%s'", vol.Name, srv.Name)
		}
	}

	mountpoint := fmt.Sprintf("%s/%s", propagatedMountPath, req.ID)

	log.Infof("creating mountpoint %s", mountpoint)
	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		return nil, errors.Wrapf(err, "could not create mountpoint %s", mountpoint)
	}

	log.Infof("mounting '%s' on '%s'", prefixedName, mountpoint)
	// copy busybox' approach and just try everything we expect might work
	for _, fstype := range supportedFileystemTypes {
		if err := mount.Mount(vol.LinuxDevice, mountpoint, fstype, ""); err == nil {
			break
		}
		return nil, errors.Errorf("could not mount '%s' as any of %s", vol.LinuxDevice, supportedFileystemTypes)
	}

	log.Infof("successfully mounted '%s' on '%s'", prefixedName, mountpoint)

	return &volume.MountResponse{Mountpoint: mountpoint}, nil
}

func (hd *hetznerDriver) Unmount(req *volume.UnmountRequest) error {
	prefixedName := prefixName(req.Name)

	log.Infof("received unmount request for '%s' as '%s'", prefixedName, req.ID)

	vol, _, err := hd.client.Volume.GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return errors.Wrapf(err, "could not get volume '%s'", prefixedName)
	}

	mountpoint := fmt.Sprintf("%s/%s", propagatedMountPath, req.ID)

	if err := mount.Unmount(mountpoint); err != nil {
		return errors.Wrapf(err, "could not unmount '%s'", mountpoint)
	}

	log.Infof("unmounted '%s'", mountpoint)

	if err := os.Remove(mountpoint); err != nil {
		return errors.Wrapf(err, "could not remove mountpoint %s", mountpoint)
	}

	log.Infof("detaching volume '%s'", prefixedName)
	act, _, err := hd.client.Volume.Detach(context.Background(), vol)
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

	srv, _, err := hd.client.Server.GetByName(context.Background(), hostname)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get cloud server '%s'", hostname)
	}

	return srv, nil
}

func (hd *hetznerDriver) waitForAction(act *hcloud.Action) error {
	_, errs := hd.client.Action.WatchProgress(context.Background(), act)
	return <-errs
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
