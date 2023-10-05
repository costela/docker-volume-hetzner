[![Go Report Card](https://goreportcard.com/badge/github.com/costela/docker-volume-hetzner)](https://goreportcard.com/report/github.com/costela/docker-volume-hetzner)
![tests](https://github.com/costela/docker-volume-hetzner/actions/workflows/main.yaml/badge.svg)

# Docker Volume Plugin for Hetzner Cloud

This plugin manages docker volumes using Hetzner Cloud's volumes.

**This plugin is still in ALPHA; use at your own risk**

## Installation

To install the plugin, run the following command:
```shell
$ docker plugin install --alias hetzner ghcr.io/costela/docker-volume-hetzner
```

When using Docker Swarm, this should be done on all nodes in the cluster.

**Important**: the plugin expects the Docker node's `hostname` to match with the name of the server created on Hetzner Cloud. This should usually be the case, unless explicitly changed.

#### Plugin privileges

During installation, you will be prompted to accept the plugins's privilege requirements. The following are required:

- **network**: used for communicating with the Hetzner Cloud API
- **mount[\/dev\/]**: needed for accessing the Hetzner Cloud Volumes (made available to the host as a SCSI device)
- **allow-all-devices**: actually enable access to the volume devices mentioned above (since the devices cannot be known a priori)
- **capabilities[CAP\_SYS\_ADMIN,CAP\_CHOWN]**: needed for running `mount` and `chown`

## Usage

First, create an API key from the Hetzner Cloud console and save it temporarily.

Install the plugin as described above. Then, set the API key in the plugin options, where `<apikey>` is the key you just created:

```shell
$ docker plugin disable hetzner
$ docker plugin set hetzner apikey=<apikey>
$ docker plugin enable hetzner
```

Again, when using Docker Swarm, this should be done on all nodes in the cluster.

The plugin is then ready to be used, e.g. in a `docker-compose` file, by setting the `driver` option on the docker `volume` definition (assuming the alias `hetzner` passed during installation above).

For example, when using the following `docker-compose` volume definition in a project called `foo`:

```yaml
volumes:
  somevolume:
    driver: hetzner
```

This will initialize a Hetzner volume named `docker-foo_somevolume` (see the `prefix` configuration below).

If the volume `docker-foo_somevolume` does not exist in the Hetzner Cloud project, the plugin will do the following:

1. Create the Hetzner Cloud (HC) volume
2. Attach the created HC volume to the node requesting the creation (when using docker swarm, this will be the manager node being used)
3. Format the HC volume (using `fstype` option; see below)
4. `chown` the volume to the appropriate `uid`/`gid` if specified.

The plugin will then mount the volume on the node running its parent service, if any.

## Configuration

The following options can be passed to the plugin via `docker plugin set` (all names **case-sensitive**):

- **`apikey`** (**required**): authentication token to use when accessing the Hetzner Cloud API
- **`size`** (optional): size of the volume in GB (default: `10`)
- **`fstype`** (optional): filesystem type to be created on new volumes. Currently supported values are `ext{2,3,4}` and `xfs` (default: `ext4`)
- **`prefix`** (optional): prefix to use when naming created volumes; the final name on the HC side will be of the form `prefix-name`, where `name` is the volume name assigned by `docker` (default: `docker`)
- **`loglevel`** (optional): the amount of information that will be output by the plugin. Accepts any value supported by [logrus](https://github.com/sirupsen/logrus) (i.e.: `fatal`, `error`, `warn`, `info` and `debug`; default: `warn`)
- **`use_protection`** (optional): whether to enable/disable deletion protection on creation/deletion. Disable this if you want to manage deletion protection yourself. (default: `true`)
- **`uid`** (optional): which user id to use by default as owners for the filesystem of newly created volumes
- **`gid`** (optional): which group id to use by default as owners for the filesystem of newly created volumes

Additionally, `size`, `fstype`, `uid` and `gid` can also be passed as options to the driver via `driver_opts`:

```yaml
volumes:
  somevolume:
    driver: hetzner
    driver_opts:
      size: '42'
      fstype: xfs
      uid: '999'
      gid: '999'
```

:warning: Passing any option besides `size`, `fstype`, `uid` and `gid` to the volume definition will have no effect beyond a warning in the logs. Use `docker plugin set` instead.

## Limitations

- *Concurrent use*: Hetzner Cloud volumes currently cannot be attached to multiple nodes, so the same limitation
applies to the docker volumes using them. This also precludes concurrent use by multiple containers on the same node,
since there is currently no way to enforce docker swarm services to be managed together (cf. kubernetes pods).
- *Single location*: since volumes are currently bound to the location they were created in, this plugin will not
be able to reattach a volume if you have a swarm cluster across locations and its service migrates over the location
boundary.
- *Volume resizing*: docker has no support for updating volume definitions. After a volume is created, its `size`
option is currently ignored. This may be worked around in a future release.
- *Docker partitions*: when used in a docker swarm setup, there is a chance a network hiccup between docker nodes
might be seen as a node down, in which case the scheduler will start the container on a different node and will
"steal" its volume while in use, potentially causing data loss.
