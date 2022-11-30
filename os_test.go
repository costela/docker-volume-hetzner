package main

import (
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/docker/docker/pkg/mount"
	"github.com/sirupsen/logrus"
)

func Test_setPermissions(t *testing.T) {
	t.Run("test", func(t *testing.T) {
		if got := setPermissions("none", "tmpfs", 33, 33, "size=1%"); got != nil {
			t.Errorf("setPermissions() = %v, want %v", got, nil)
		}
	})
}

func Test_chown(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "mnt-*")
	if err != nil {
		t.Errorf("failed creating temp dir")
	}

	if err := mount.Mount("none", tmpDir, "tmpfs", "size=1%"); err != nil {
		t.Errorf("failed tempMount")
	}

	if err := chownIfEmpty(tmpDir, 33, 33); err != nil {
		// clean up
		if unmountErr := mount.Unmount(tmpDir); unmountErr != nil {
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

	if err := mount.Unmount(tmpDir); err != nil {
		t.Errorf("failed unmount command")
	}

	if uid != 33 {
		t.Errorf("mount had wrong uid, got %d", uid)
	}

	if gid != 33 {
		t.Errorf("mount had wrong gid, got %d", gid)
	}
}

func Test_chownIfEmptyIgnoresLostAndFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "mnt-*")
	if err != nil {
		t.Errorf("failed creating temp dir")
	}

	if err := mount.Mount("none", tmpDir, "tmpfs", "size=1%"); err != nil {
		t.Errorf("failed tempMount")
	}

	os.MkdirAll(fmt.Sprintf("%s/lost+found", tmpDir), 0644)
	if err := chownIfEmpty(tmpDir, 33, 33); err != nil {
		// clean up
		if unmountErr := mount.Unmount(tmpDir); unmountErr != nil {
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

	if err := mount.Unmount(tmpDir); err != nil {
		t.Errorf("failed unmount command")
	}

	if uid != 33 {
		t.Errorf("mount had wrong uid, got %d", uid)
	}

	if gid != 33 {
		t.Errorf("mount had wrong gid, got %d", gid)
	}
}

func Test_chownIfEmptyWithNonEmptyDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "mnt-*")
	if err != nil {
		t.Errorf("failed creating temp dir")
	}

	if err := mount.Mount("none", tmpDir, "tmpfs", "size=1%"); err != nil {
		t.Errorf("failed tempMount")
	}

	os.WriteFile(fmt.Sprintf("%s/somefile.txt", tmpDir), []byte("hello\ngo\n"), 0644)
	errAfterFileCreated := chownIfEmpty(tmpDir, 34, 34)

	var uid uint32
	var gid uint32

	info, err := os.Stat(tmpDir)
	if err == nil {
		stat := info.Sys().(*syscall.Stat_t)
		uid = stat.Uid
		gid = stat.Gid
	}

	if err := mount.Unmount(tmpDir); err != nil {
		t.Errorf("failed unmount command")
	}

	if errAfterFileCreated == nil {
		t.Errorf("chownIfEmpty succeeded even though file was in directory")
	}

	if uid != 0 {
		t.Errorf("mount had wrong uid, got %d", uid)
	}

	if gid != 0 {
		t.Errorf("mount had wrong gid, got %d", gid)
	}
}
