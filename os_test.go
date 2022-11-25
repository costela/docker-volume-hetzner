package main

import (
	"os"
	"syscall"
	"testing"

	"github.com/sirupsen/logrus"
)

func Test_setPermissions(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		if got := setPermissions("none", "tmpfs", "33:33", "-o", "size=1%"); got != nil {
			t.Errorf("setPermissions() = %v, want %v", got, nil)
		}
	})
}

func Test_chown(t *testing.T) {
	tmpDir := os.TempDir()

	if err := tempMount("none", "tmpfs", "-o", "size=1%"); err != nil {
		t.Errorf("failed tempMount")
	}

	if err := chown(tmpDir, "33:33"); err != nil {
		// clean up
		if umountErr := umount(tmpDir); umountErr != nil {
			logrus.Errorf("failed unmounting while cleaning up after error in chown")
		}
		t.Errorf("failed chown command")
	}

	var uid uint32
	var gid uint32

	info, err := os.Stat(tmpDir)
	if err == nil {
		stat := info.Sys().(*syscall.Stat_t)
		uid = stat.Uid
		gid = stat.Gid
	}

	if err := umount(tmpDir); err != nil {
		t.Errorf("failed umount command")
	}

	if uid != 33 {
		t.Errorf("mount had wrong uid, got %d", uid)
	}

	if gid != 33 {
		t.Errorf("mount had wrong gid, got %d", gid)
	}
}
