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

func umount(mntpoint string) error {
	tmpDir := os.TempDir()
	var stderr bytes.Buffer

	cmd := exec.Command(
		"/bin/umount",
		tmpDir,
	)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logrus.Errorf("umount stderr: %s", stderr.String())
		return err
	}
	return nil
}

func tempMount(dev, fstype string, mountOptions ...string) error {
	tmpDir := os.TempDir()
	var stderr bytes.Buffer

	mountArgs := []string{
		"-t",
		fstype,
	}
	mountArgs = append(mountArgs, mountOptions...)
	mountArgs = append(mountArgs,
		dev,
		tmpDir,
	)
	cmd := exec.Command(
		"/bin/mount",
		mountArgs...,
	)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logrus.Errorf("mount stderr: %s", stderr.String())
		return err
	}
	return nil
}

func chown(dir string, uid uint32, gid uint32) error {
	uidgid := fmt.Sprintf("%d:%d", uid, gid)
	var stderr bytes.Buffer
	cmd := exec.Command(
		"/bin/chown",
		"-R",
		uidgid,
		dir,
	)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logrus.Errorf("chown stderr: %s", stderr.String())
		return err
	}
	return nil
}

func setPermissions(dev, fstype string, uid uint32, gid uint32, mountOptions ...string) error {
	tmpDir := os.TempDir()

	if err := tempMount(dev, fstype, mountOptions...); err != nil {
		// nothing to clean up yet
		return err
	}

	if err := chown(tmpDir, uid, gid); err != nil {
		// clean up
		if umountErr := umount(tmpDir); umountErr != nil {
			logrus.Errorf("failed unmounting while cleaning up after error in chown")
		}
		return err
	}

	return umount(tmpDir)
}
