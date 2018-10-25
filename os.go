package main

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/docker/docker/pkg/mount"
	log "github.com/sirupsen/logrus"
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
		log.Errorf("mkfs stderr: %s", stderr.String())
		return err
	}
	return nil
}
