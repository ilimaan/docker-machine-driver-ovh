package main

import (
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/drivers/plugin"
)

// Default values for docker-machine-driver-ovh
const (
	DefaultSecurityGroup = "default"
	DefaultProjectName   = "docker-machine"
	DefaultFlavorName    = "b2-7"
	DefaultRegionName    = "GRA1"
	DefaultImageName     = "Ubuntu 20.04"
	DefaultSSHUserName   = "ubuntu"
	DefaultBillingPeriod = "hourly"
)

func main() {
	plugin.RegisterDriver(&Driver{
		BaseDriver: &drivers.BaseDriver{
			SSHUser: DefaultSSHUserName,
			SSHPort: 22,
		}})
}
