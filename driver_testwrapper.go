package main

import (
	"context"

	"github.com/hetznercloud/hcloud-go/hcloud"
)

// these types wrap hcloud.Client to make mocking easier
type hetznerClienter interface {
	Volume() hetznerVolumeClienter
	Server() hetznerServerClienter
	Action() hetznerActionClienter
}

type hetznerVolumeClienter interface {
	All(context.Context) ([]*hcloud.Volume, error)
	Attach(context.Context, *hcloud.Volume, *hcloud.Server) (*hcloud.Action, *hcloud.Response, error)
	ChangeProtection(context.Context, *hcloud.Volume, hcloud.VolumeChangeProtectionOpts) (*hcloud.Action, *hcloud.Response, error)
	Create(context.Context, hcloud.VolumeCreateOpts) (hcloud.VolumeCreateResult, *hcloud.Response, error)
	Delete(context.Context, *hcloud.Volume) (*hcloud.Response, error)
	Detach(context.Context, *hcloud.Volume) (*hcloud.Action, *hcloud.Response, error)
	GetByName(context.Context, string) (*hcloud.Volume, *hcloud.Response, error)
}

type hetznerServerClienter interface {
	GetByID(ctx context.Context, id int) (*hcloud.Server, *hcloud.Response, error)
	GetByName(ctx context.Context, name string) (*hcloud.Server, *hcloud.Response, error)
}

type hetznerActionClienter interface {
	WatchProgress(ctx context.Context, action *hcloud.Action) (<-chan int, <-chan error)
}

type hetznerClient struct {
	client *hcloud.Client
}

func (h *hetznerClient) Volume() hetznerVolumeClienter {
	return &h.client.Volume
}

func (h *hetznerClient) Server() hetznerServerClienter {
	return &h.client.Server
}

func (h *hetznerClient) Action() hetznerActionClienter {
	return &h.client.Action
}
