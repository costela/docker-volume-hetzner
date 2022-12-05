package main

import (
	"os"
	"testing"

	"github.com/docker/go-plugins-helpers/volume"
)

func TestMain(m *testing.M) {
	for k, v := range map[string]string{
		"prefix": "docker",
		"fstype": "ext4",
		"size":   "10",
	} {
		os.Setenv(k, v)
	}
	os.Exit(m.Run())
}

func Test_getOption(t *testing.T) {
	type args struct {
		k    string
		opts map[string]string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"opts first", args{k: "opt", opts: map[string]string{"opt": "bar"}}, "bar"},
		{"env fallback", args{k: "prefix", opts: map[string]string{"opt": "bar"}}, "docker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getOption(tt.args.k, tt.args.opts); got != tt.want {
				t.Errorf("getOption() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_prefixName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"short", args{name: "foo"}, "docker-foo"},
		{
			"long", // API limits it to 64
			args{name: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			"docker-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prefixName(tt.args.name); got != tt.want {
				t.Errorf("prefixName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_unprefixedName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"remove prefix", args{name: "docker-foobar"}, "foobar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unprefixedName(tt.args.name); got != tt.want {
				t.Errorf("unprefixedName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_hetznerDriver_Create(t *testing.T) {
	type fields struct {
		client hetznerClienter
	}
	type args struct {
		req *volume.CreateRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hd := &hetznerDriver{
				client: tt.fields.client,
			}
			if err := hd.Create(tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("hetznerDriver.Create() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// func Test_hetznerDriver_List(t *testing.T) {
// 	type fields struct {
// 		client *hcloud.Client
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		want    *volume.ListResponse
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			hd := &hetznerDriver{
// 				client: tt.fields.client,
// 			}
// 			got, err := hd.List()
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("hetznerDriver.List() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("hetznerDriver.List() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func Test_hetznerDriver_Get(t *testing.T) {
// 	type fields struct {
// 		client *hcloud.Client
// 	}
// 	type args struct {
// 		req *volume.GetRequest
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		want    *volume.GetResponse
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			hd := &hetznerDriver{
// 				client: tt.fields.client,
// 			}
// 			got, err := hd.Get(tt.args.req)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("hetznerDriver.Get() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("hetznerDriver.Get() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func Test_hetznerDriver_Remove(t *testing.T) {
// 	type fields struct {
// 		client *hcloud.Client
// 	}
// 	type args struct {
// 		req *volume.RemoveRequest
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			hd := &hetznerDriver{
// 				client: tt.fields.client,
// 			}
// 			if err := hd.Remove(tt.args.req); (err != nil) != tt.wantErr {
// 				t.Errorf("hetznerDriver.Remove() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

// func Test_hetznerDriver_Path(t *testing.T) {
// 	type fields struct {
// 		client *hcloud.Client
// 	}
// 	type args struct {
// 		req *volume.PathRequest
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		want    *volume.PathResponse
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			hd := &hetznerDriver{
// 				client: tt.fields.client,
// 			}
// 			got, err := hd.Path(tt.args.req)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("hetznerDriver.Path() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("hetznerDriver.Path() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func Test_hetznerDriver_Mount(t *testing.T) {
// 	type fields struct {
// 		client *hcloud.Client
// 	}
// 	type args struct {
// 		req *volume.MountRequest
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		want    *volume.MountResponse
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			hd := &hetznerDriver{
// 				client: tt.fields.client,
// 			}
// 			got, err := hd.Mount(tt.args.req)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("hetznerDriver.Mount() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("hetznerDriver.Mount() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func Test_hetznerDriver_Unmount(t *testing.T) {
// 	type fields struct {
// 		client *hcloud.Client
// 	}
// 	type args struct {
// 		req *volume.UnmountRequest
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			hd := &hetznerDriver{
// 				client: tt.fields.client,
// 			}
// 			if err := hd.Unmount(tt.args.req); (err != nil) != tt.wantErr {
// 				t.Errorf("hetznerDriver.Unmount() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

// func Test_hetznerDriver_getServerForLocalhost(t *testing.T) {
// 	type fields struct {
// 		client *hcloud.Client
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		want    *hcloud.Server
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			hd := &hetznerDriver{
// 				client: tt.fields.client,
// 			}
// 			got, err := hd.getServerForLocalhost()
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("hetznerDriver.getServerForLocalhost() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("hetznerDriver.getServerForLocalhost() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }
