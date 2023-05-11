package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/docker/docker/pkg/mount"
	"github.com/sirupsen/logrus"
)

var supportedFileystemTypes = [...]string{"ext4", "xfs", "ext3", "ext2"}

func getMounts() (map[string]string, error) {
	mounts, err := mount.GetMounts()
	if err != nil {
		return nil, err
	}
	mountsMap := make(map[string]string, len(mounts))
	for _, mount := range mounts {
		mountsMap[mount.Source] = mount.Mountpoint
	}
	return mountsMap, nil
}

func mkfs(dev, fstype string) error {
	mkfsExec := fmt.Sprintf("/sbin/mkfs.%s", fstype)
	cmd := exec.Command(mkfsExec, dev)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logrus.Errorf("mkfs stderr: %s", stderr.String())
		return err
	}
	return nil
}

func setPermissions(dev, fstype string, uid int, gid int) (err error) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "mnt-*")
	if err != nil {
		return fmt.Errorf("creating temp dir for chmod: %w", err)
	}

	if err := mount.Mount(
		dev,
		tmpDir,
		fstype,
		"",
	); err != nil {
		// nothing to clean up yet
		return fmt.Errorf("mounting: %w", err)
	}

	defer func() {
		// clean up
		if unmountErr := mount.Unmount(tmpDir); err == nil && unmountErr != nil {
			err = fmt.Errorf("unmounting after chown: %w", unmountErr)
			return
		}

		if rmErr := os.Remove(tmpDir); err == nil && rmErr != nil {
			err = rmErr
		}
	}()

	if err := os.Chown(tmpDir, uid, gid); err != nil {
		return fmt.Errorf("chowning: %w", err)
	}

	return nil
}
