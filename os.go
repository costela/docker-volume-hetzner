package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
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

func chownIfEmpty(dir string, uid int, gid int) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, file := range files {
		isLostAndFound := file.Name() == "lost+found" && file.IsDir()
		if !isLostAndFound {
			return fmt.Errorf("dir is not an empty Hetzner Volume. Will not chown.")
		}
	}

	if err := os.Chown(
		dir,
		uid,
		gid,
	); err != nil {
		logrus.Errorf("chown error: %s", err)
		return err
	}
	return nil
}

func setPermissions(dev, fstype string, uid int, gid int, mountOptions string) error {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "mnt-*")
	if err != nil {
		return fmt.Errorf("failed creating temp dir for setting permissions")
	}

	if err := mount.Mount(
		dev,
		tmpDir,
		fstype,
		mountOptions,
	); err != nil {
		// nothing to clean up yet
		return err
	}

	if err := os.Chown(tmpDir, uid, gid); err != nil {
		// clean up
		if unmountErr := mount.Unmount(tmpDir); unmountErr != nil {
			logrus.Errorf("failed unmounting while cleaning up after error in chown")
		}
		return err
	}

	return mount.Unmount(tmpDir)
}
