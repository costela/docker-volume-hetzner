package main

import (
	"testing"
)

func Test_setPermissions(t *testing.T) {
	if got := setPermissions("none", "tmpfs", 33, 33); got != nil {
		t.Errorf("setPermissions() = %v, want %v", got, nil)
	}
}
