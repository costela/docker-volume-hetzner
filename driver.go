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
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sirupsen/logrus"

	"github.com/docker/go-plugins-helpers/volume"
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

	logrus.Infof("starting volume creation for %q", prefixedName)

	size, err := strconv.Atoi(getOption("size", req.Options))
	if err != nil {
		return fmt.Errorf("converting size %q to int: %w", getOption("size", req.Options), err)
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
	switch f := getOption("fstype", req.Options); f {
	case "xfs", "ext4":
		opts.Format = hcloud.String(f)
	}

	resp, _, err := hd.client.Volume().Create(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("creating volume %q: %w", prefixedName, err)
	}
	if err := hd.waitForAction(resp.Action); err != nil {
		return fmt.Errorf("waiting for create volume %q: %w", prefixedName, err)
	}

	logrus.Infof("volume %q (%dGB) created on %q; attaching", prefixedName, size, srv.Name)

	act, _, err := hd.client.Volume().Attach(context.Background(), resp.Volume, srv)
	if err != nil {
		return fmt.Errorf("attaching volume %q to %q: %w", prefixedName, srv.Name, err)
	}
	if err := hd.waitForAction(act); err != nil {
		return fmt.Errorf("waiting for volume attachment: %q to %q: %w", prefixedName, srv.Name, err)
	}

	logrus.Infof("volume %q attached to %q", prefixedName, srv.Name)

	if useProtection() {
		// be optimistic for now and ignore errors here
		_, _, _ = hd.client.Volume().ChangeProtection(context.Background(), resp.Volume, hcloud.VolumeChangeProtectionOpts{Delete: &trueVar})
	}

	if opts.Format == nil {
		logrus.Infof("formatting %q as %q", prefixedName, getOption("fstype", req.Options))
		err = mkfs(resp.Volume.LinuxDevice, getOption("fstype", req.Options))
		if err != nil {
			return fmt.Errorf("mkfs on %q: %w", resp.Volume.LinuxDevice, err)
		}
	}

	uid := getOption("uid", req.Options)
	gid := getOption("gid", req.Options)
	if uid != "0" || gid != "0" {
		// string to int
		uintParsed, err := strconv.Atoi(uid)
		if err != nil {
			return fmt.Errorf("parsing uid option value as integer: %s: %w", gid, err)
		}
		gidParsed, err := strconv.Atoi(gid)
		if err != nil {
			return fmt.Errorf("parsing gid option value as integer: %s: %w", gid, err)
		}

		if err := setPermissions(resp.Volume.LinuxDevice, getOption("fstype", req.Options), uintParsed, gidParsed); err != nil {
			return fmt.Errorf("chown %q to '%s:%s': %w", resp.Volume.LinuxDevice, uid, gid, err)
		}
	}

	return nil
}

func (hd *hetznerDriver) List() (*volume.ListResponse, error) {
	logrus.Infof("got list request")

	vols, err := hd.client.Volume().All(context.Background())
	if err != nil {
		return nil, fmt.Errorf("could not list all volumes: %w", err)
	}

	mounts, err := getMounts()
	if err != nil {
		return nil, fmt.Errorf("could not get local mounts: %w", err)
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

	logrus.Infof("fetching information for volume %q", prefixedName)

	vol, _, err := hd.client.Volume().GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return nil, fmt.Errorf("getting cloud volume %q: %w", prefixedName, err)
	}

	mounts, err := getMounts()
	if err != nil {
		return nil, fmt.Errorf("getting local mounts: %w", err)
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

	logrus.Infof("returning info on %q: %#v", prefixedName, resp.Volume)

	return &resp, nil
}

func (hd *hetznerDriver) Remove(req *volume.RemoveRequest) error {
	prefixedName := prefixName(req.Name)

	logrus.Infof("starting volume removal for %q", prefixedName)

	vol, _, err := hd.client.Volume().GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return fmt.Errorf("getting cloud volume %q: %w", prefixedName, err)
	}

	if useProtection() {
		logrus.Infof("disabling protection for %q", prefixedName)
		act, _, err := hd.client.Volume().ChangeProtection(context.Background(), vol, hcloud.VolumeChangeProtectionOpts{Delete: &falseVar})
		if err != nil {
			return fmt.Errorf("unprotecting volume %q: %w", prefixedName, err)
		}
		if err := hd.waitForAction(act); err != nil {
			return fmt.Errorf("waiting for volume unprotecton %q: %w", prefixedName, err)
		}
	}

	if vol.Server != nil && vol.Server.ID != 0 {
		logrus.Infof("detaching volume %q (attached to %d)", prefixedName, vol.Server.ID)
		act, _, err := hd.client.Volume().Detach(context.Background(), vol)
		if err != nil {
			return fmt.Errorf("detaching volume %q: %w", prefixedName, err)
		}
		if err := hd.waitForAction(act); err != nil {
			return fmt.Errorf("waiting for volume detach on %q: %w", prefixedName, err)
		}
	}

	_, err = hd.client.Volume().Delete(context.Background(), vol)
	if err != nil {
		return fmt.Errorf("deleting volume %q: %w", prefixedName, err)
	}

	logrus.Infof("volume %q removed successfully", prefixedName)

	return nil
}

func (hd *hetznerDriver) Path(req *volume.PathRequest) (*volume.PathResponse, error) {
	prefixedName := prefixName(req.Name)

	logrus.Infof("got path request for volume %q", prefixedName)

	resp, err := hd.Get(&volume.GetRequest{Name: req.Name})
	if err != nil {
		return nil, fmt.Errorf("getting path for volume %q: %w", prefixedName, err)
	}

	return &volume.PathResponse{Mountpoint: resp.Volume.Mountpoint}, nil
}

func (hd *hetznerDriver) Mount(req *volume.MountRequest) (*volume.MountResponse, error) {
	prefixedName := prefixName(req.Name)

	logrus.Infof("received mount request for %q as %q", prefixedName, req.ID)

	vol, _, err := hd.client.Volume().GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return nil, fmt.Errorf("getting volume %q: %w", prefixedName, err)
	}

	if vol.Server != nil && vol.Server.ID != 0 {
		volSrv, _, err := hd.client.Server().GetByID(context.Background(), vol.Server.ID)
		if err != nil {
			return nil, fmt.Errorf("fetching server details for volume %q: %w", prefixedName, err)
		}
		vol.Server = volSrv
	}

	srv, err := hd.getServerForLocalhost()
	if err != nil {
		return nil, err
	}

	if vol.Server == nil || vol.Server.Name != srv.Name {
		if vol.Server != nil && vol.Server.Name != "" {
			logrus.Infof("detaching volume %q from %q", prefixedName, vol.Server.Name)
			act, _, err := hd.client.Volume().Detach(context.Background(), vol)
			if err != nil {
				return nil, fmt.Errorf("detaching volume %q from %q: %w", vol.Name, vol.Server.Name, err)
			}
			if err := hd.waitForAction(act); err != nil {
				return nil, fmt.Errorf("waiting for volume detachment on %q from %q: %w", vol.Name, vol.Server.Name, err)
			}
		}
		logrus.Infof("attaching volume %q to %q", prefixedName, srv.Name)
		act, _, err := hd.client.Volume().Attach(context.Background(), vol, srv)
		if err != nil {
			return nil, fmt.Errorf("attaching volume %q to %q: %w", vol.Name, srv.Name, err)
		}
		if err := hd.waitForAction(act); err != nil {
			return nil, fmt.Errorf("waiting for volume attachment on %q to %q: %w", vol.Name, srv.Name, err)
		}
	}

	mountpoint := fmt.Sprintf("%s/%s", propagatedMountPath, req.ID)

	logrus.Infof("creating mountpoint %s", mountpoint)
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return nil, fmt.Errorf("creating mountpoint %s: %w", mountpoint, err)
	}

	logrus.Infof("mounting %q on %q", prefixedName, mountpoint)

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
		return nil, fmt.Errorf("mounting %q as any of %s: %w", vol.LinuxDevice, supportedFileystemTypes, err)
	}

	logrus.Infof("successfully mounted %q on %q", prefixedName, mountpoint)

	return &volume.MountResponse{Mountpoint: mountpoint}, nil
}

func (hd *hetznerDriver) Unmount(req *volume.UnmountRequest) error {
	prefixedName := prefixName(req.Name)

	logrus.Infof("received unmount request for %q as %q", prefixedName, req.ID)

	vol, _, err := hd.client.Volume().GetByName(context.Background(), prefixedName)
	if err != nil || vol == nil {
		return fmt.Errorf("getting volume %q: %w", prefixedName, err)
	}

	mountpoint := fmt.Sprintf("%s/%s", propagatedMountPath, req.ID)

	if err := mount.Unmount(mountpoint); err != nil {
		return fmt.Errorf("unmounting %q: %w", mountpoint, err)
	}

	logrus.Infof("unmounted %q", mountpoint)

	if err := os.Remove(mountpoint); err != nil {
		return fmt.Errorf("removing mountpoint %s: %w", mountpoint, err)
	}

	srv, err := hd.getServerForLocalhost()
	if err != nil {
		return nil
	}

	if vol.Server == nil || vol.Server.Name != srv.Name {
		return nil
	}

	logrus.Infof("detaching volume %q", prefixedName)

	act, _, err := hd.client.Volume().Detach(context.Background(), vol)
	if err != nil {
		return fmt.Errorf("detaching volume %q: %w", vol.Name, err)
	}
	if err := hd.waitForAction(act); err != nil {
		return fmt.Errorf("waiting for volume detach on %q: %w", vol.Name, err)
	}

	return nil
}

func (hd *hetznerDriver) getServerForLocalhost() (*hcloud.Server, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("getting local hostname: %w", err)
	}

	if strings.Contains(hostname, ".") {
		logrus.Warnf("hostname contains dot (%q); make sure hostname != FQDN and matches the hcloud server name", hostname)
	}

	srv, _, err := hd.client.Server().GetByName(context.Background(), hostname)
	if err != nil {
		return nil, fmt.Errorf("getting cloud server %q: %w", hostname, err)
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
		case "fstype", "size", "uid", "gid": // OK, noop
		default:
			logrus.Warnf("unsupported driver_opt %q for volume %s", k, volume)
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
