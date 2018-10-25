PLUGIN_NAME = costela/docker-volume-hetzner
PLUGIN_TAG ?= dev

all: create

clean:
	@rm -rf ./plugin
	@docker container rm -vf tmp_plugin_build || true

rootfs: clean
	docker image build -t ${PLUGIN_NAME}:rootfs .
	mkdir -p ./plugin/rootfs
	docker container create --name tmp_plugin_build ${PLUGIN_NAME}:rootfs
	docker container export tmp_plugin_build | tar -x -C ./plugin/rootfs
	cp config.json ./plugin/
	docker container rm -vf tmp_plugin_build

create: rootfs
	docker plugin rm -f ${PLUGIN_NAME}:${PLUGIN_TAG} || true
	docker plugin create ${PLUGIN_NAME}:${PLUGIN_TAG} ./plugin

enable: create
	docker plugin enable ${PLUGIN_NAME}:${PLUGIN_TAG}

push:  create
	docker plugin push ${PLUGIN_NAME}:${PLUGIN_TAG}

.PHONY: clean rootfs create