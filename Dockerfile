FROM golang:1.11-alpine as builder

ENV CGO_ENABLED=0

RUN apk add --update git

WORKDIR /plugin

# warm up go mod cache
COPY go.mod go.sum ./
RUN go mod download

COPY . /plugin
RUN go build -v


FROM alpine

RUN apk add --update ca-certificates e2fsprogs xfsprogs

RUN mkdir -p /run/docker/plugins /mnt/volumes

COPY --from=builder /plugin/docker-volume-hetzner /plugin/

ENTRYPOINT [ "/plugin/docker-volume-hetzner" ]
