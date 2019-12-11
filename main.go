package main // import "github.com/costela/docker-volume-hetzner"

import (
	"fmt"
	"os"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/sirupsen/logrus"
)

const socketAddress = "/run/docker/plugins/hetzner.sock"
const propagatedMountPath = "/mnt"

func main() {
	logrus.SetFormatter(&bareFormatter{})

	logLevel, err := logrus.ParseLevel(os.Getenv("loglevel"))
	if err != nil {
		logrus.Fatalf("could not parse log level %s", os.Getenv("loglevel"))
	}

	logrus.SetLevel(logLevel)

	hd := newHetznerDriver()
	h := volume.NewHandler(hd)
	logrus.Infof("listening on %s", socketAddress)
	if err := h.ServeUnix(socketAddress, 0); err != nil {
		logrus.Fatalf("error serving docker socket: %v", err)
	}
}

type bareFormatter struct{}

func (bareFormatter) Format(e *logrus.Entry) ([]byte, error) {
	return []byte(fmt.Sprintf("%s\n", e.Message)), nil
}
