package main // import "github.com/costela/docker-volume-hetzner"

import (
	"fmt"
	"os"

	"github.com/docker/go-plugins-helpers/volume"
	log "github.com/sirupsen/logrus"
)

const socketAddress = "/run/docker/plugins/hetzner.sock"
const propagatedMountPath = "/mnt"

func main() {
	// logFile, err := os.Create("/var/log/plugin.log")
	// if err != nil {
	// 	panic("could not open logfile")
	// }
	// defer logFile.Close()
	// log.SetOutput(logFile)

	log.SetFormatter(&bareFormatter{})

	logLevel, err := log.ParseLevel(os.Getenv("loglevel"))
	if err != nil {
		log.Fatalf("could not parse log level %s", os.Getenv("loglevel"))
	}

	log.SetLevel(logLevel)

	hd := newHetznerDriver()
	h := volume.NewHandler(hd)
	log.Infof("listening on %s", socketAddress)
	log.Error(h.ServeUnix(socketAddress, 0))
}

type bareFormatter struct{}

func (bareFormatter) Format(e *log.Entry) ([]byte, error) {
	return []byte(fmt.Sprintf("%s\n", e.Message)), nil
}
