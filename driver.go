package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/mcnutils"
	"github.com/docker/machine/libmachine/ssh"
	"github.com/docker/machine/libmachine/state"
)

const (
	statusTimeout = 200
)

// Driver is a machine driver for OVH.
type Driver struct {
	*drivers.BaseDriver

	// Command line parameters
	ProjectName        string
	FlavorName         string
	RegionName         string
	PrivateNetworkName string

	// Ovh specific parameters
	BillingPeriod string
	Endpoint      string

	// Internal ids
	ProjectID   string
	FlavorID    string
	ImageID     string
	InstanceID  string
	KeyPairName string
	KeyPairID   string
	NetworkIDs  []string

	// Overloaded credentials
	ApplicationKey    string
	ApplicationSecret string
	ConsumerKey       string

	// internal
	client *API
}

// GetCreateFlags registers the "machine create" flags recognized by this driver, including
// their help text and defaults.
func (d *Driver) GetCreateFlags() []mcnflag.Flag {
	return []mcnflag.Flag{
		mcnflag.StringFlag{
			EnvVar: "OVH_APPLICATION_KEY",
			Name:   "ovh-application-key",
			Usage:  "OVH API application key. May be stored in ovh.conf",
			Value:  "",
		},
		mcnflag.StringFlag{
			EnvVar: "OVH_APPLICATION_SECRET",
			Name:   "ovh-application-secret",
			Usage:  "OVH API application secret. May be stored in ovh.conf",
			Value:  "",
		},
		mcnflag.StringFlag{
			EnvVar: "OVH_CONSUMER_KEY",
			Name:   "ovh-consumer-key",
			Usage:  "OVH API consumer key. May be stored in ovh.conf",
			Value:  "",
		},
		mcnflag.StringFlag{
			Name:  "ovh-endpoint",
			Usage: "OVH Cloud API endpoint. Default: ovh-eu",
			Value: "",
		},
		mcnflag.StringFlag{
			Name:  "ovh-project",
			Usage: "OVH Cloud project name or id",
			Value: "",
		},
		mcnflag.StringFlag{
			Name:  "ovh-region",
			Usage: "OVH Cloud region name",
			Value: DefaultRegionName,
		},
		mcnflag.StringFlag{
			Name:  "ovh-flavor",
			Usage: "OVH Cloud flavor name or id. Default: b2-7",
			Value: DefaultFlavorName,
		},
		mcnflag.StringFlag{
			Name:  "ovh-image",
			Usage: "OVH Cloud Image name or id. Default: Ubuntu 20.04",
			Value: DefaultImageName,
		},
		mcnflag.StringFlag{
			Name:  "ovh-private-network",
			Usage: "OVH Cloud (private) network name or vlan number. Default: public network",
			Value: "",
		},
		mcnflag.StringFlag{
			Name:  "ovh-ssh-key",
			Usage: "OVH Cloud ssh key name or id to use. Default: generate a random name",
			Value: "",
		},
		mcnflag.StringFlag{
			Name:  "ovh-ssh-user",
			Usage: "OVH Cloud ssh username to use. Default: machine",
			Value: DefaultSSHUserName,
		},
		mcnflag.StringFlag{
			Name:  "ovh-billing-period",
			Usage: "OVH Cloud billing period (hourly or monthly). Default: hourly",
			Value: DefaultBillingPeriod,
		},
	}
}

// DriverName returns the name of the driver
func (d *Driver) DriverName() string {
	return "ovh"
}

// getClient returns an OVH API client
func (d *Driver) getClient() (api *API, err error) {
	if d.client == nil {
		client, err := NewAPI(d.Endpoint, d.ApplicationKey, d.ApplicationSecret, d.ConsumerKey)
		if err != nil {
			return nil, fmt.Errorf("Could not create a connection to OVH API. You may want to visit: https://github.com/yadutaf/docker-machine-driver-ovh#example-usage. The original error was: %s", err)
		}
		d.client = client
	}

	return d.client, nil
}

// SetConfigFromFlags assigns and verifies the command-line arguments presented to the driver.
func (d *Driver) SetConfigFromFlags(flags drivers.DriverOptions) error {
	d.ApplicationKey = flags.String("ovh-application-key")
	d.ApplicationSecret = flags.String("ovh-application-secret")
	d.ConsumerKey = flags.String("ovh-consumer-key")

	// Store configuration parameters as-is
	d.Endpoint = flags.String("ovh-endpoint")
	d.ProjectName = flags.String("ovh-project")
	d.RegionName = flags.String("ovh-region")
	d.FlavorName = flags.String("ovh-flavor")
	d.ImageID = flags.String("ovh-image")
	d.PrivateNetworkName = flags.String("ovh-private-network")
	d.KeyPairName = flags.String("ovh-ssh-key")
	d.BillingPeriod = flags.String("ovh-billing-period")

	// Swarm configuration, must be in each driver
	d.SwarmMaster = flags.Bool("swarm-master")
	d.SwarmHost = flags.String("swarm-host")
	d.SwarmDiscovery = flags.String("swarm-discovery")

	d.SSHUser = flags.String("ovh-ssh-user")

	return nil
}

// PreCreateCheck does the network side validation
func (d *Driver) PreCreateCheck() error {
	client, err := d.getClient()
	if err != nil {
		return err
	}

	// Validate billing period
	log.Debug("Validating billing period")
	if d.BillingPeriod != "monthly" && d.BillingPeriod != "hourly" {
		return fmt.Errorf("Invalid billing period '%s'. Please select one of 'hourly', 'monthly'", d.BillingPeriod)
	}
	log.Debug("Selecting billing period", d.BillingPeriod)

	// Validate project id
	log.Debug("Validating project")
	if d.ProjectName != "" {
		project, err := client.GetProjectByName(d.ProjectName)
		if err != nil {
			return err
		}
		d.ProjectID = project.ID
	} else {
		projects, err := client.GetProjects()
		if err != nil {
			return err
		}

		// If there is only one project, take it
		if len(projects) == 1 {
			d.ProjectID = projects[0]
		} else if len(projects) == 0 {
			return fmt.Errorf("No Cloud project could be found. To create a new one, please visit %s", CustomerInterface)
		} else {
			// Build a list of project names to help choose one
			var projectNames []string
			for _, projectID := range projects {
				project, err := client.GetProject(projectID)
				if err != nil {
					projectNames = append(projectNames, projectID)
				} else {
					projectNames = append(projectNames, project.Name)
				}
			}

			return fmt.Errorf("Multiple Cloud project found (%s), to select one, use '--ovh-project' option", strings.Join(projectNames[:], ", "))
		}
	}
	log.Debug("Found project id ", d.ProjectID)

	// Validate region
	log.Debug("Validating region")
	regions, err := client.GetRegions(d.ProjectID)
	if err != nil {
		return err
	}
	var ok bool
	for _, region := range regions {
		if region == d.RegionName {
			ok = true
			break
		}
	}
	if ok != true {
		return fmt.Errorf("Invalid region %s. For a list of valid ovh regions, please visis %s", d.RegionName, CustomerInterface)
	}

	// Validate flavor
	log.Debug("Validating flavor")
	flavor, err := client.GetFlavorByName(d.ProjectID, d.RegionName, d.FlavorName)
	if err != nil {
		return err
	}
	d.FlavorID = flavor.ID
	log.Debug("Found flavor id ", d.FlavorID)

	// Validate image
	log.Debug("Validating image")
	image, err := client.GetImageByName(d.ProjectID, d.RegionName, d.ImageID)
	if err != nil {
		return err
	}
	d.ImageID = image.ID
	log.Debug("Found image id ", d.ImageID)

	// Validate private network
	log.Debug("Validating private network")
	if d.PrivateNetworkName != "" {
		privateNetwork, err := client.GetPrivateNetworkByName(d.ProjectID, d.PrivateNetworkName)
		if err != nil {
			return err
		}
		d.NetworkIDs = append(d.NetworkIDs, privateNetwork.ID)
		log.Debug("Found private network id ", privateNetwork.ID)

		publicNetworkID, err := client.GetPublicNetworkID(d.ProjectID)
		if err != nil {
			return err
		}
		d.NetworkIDs = append(d.NetworkIDs, publicNetworkID)
		log.Debug("Found public network id ", publicNetworkID)

	} else {
		log.Debug("No private network found. Using public network")
	}

	// Use a common key or create a machine specific one
	keyPath := filepath.Join(d.StorePath, "sshkeys", d.KeyPairName)
	if len(d.KeyPairName) != 0 {
		if _, err := os.Stat(keyPath); err == nil {
			d.SSHKeyPath = keyPath
		} else {
			log.Debug("SSH key", keyPath, "does not exist. Assuming the key (", d.KeyPairName, ") is in '~/.ssh/' or in a SSH agent.")
		}
	} else {
		d.KeyPairName = fmt.Sprintf("%s-%s", d.MachineName, mcnutils.GenerateRandomID())
		sanitizeKeyPairName(&d.KeyPairName)
		d.SSHKeyPath = d.ResolveStorePath(d.KeyPairName)
	}

	return nil
}

// copied from openstack driver
func sanitizeKeyPairName(s *string) {
	*s = strings.Replace(*s, ".", "_", -1)
}

// ensureSSHKey makes sure an SSH key for the machine exists with requested name
func (d *Driver) ensureSSHKey() error {
	client, err := d.getClient()
	if err != nil {
		return err
	}

	// Attempt to get an existing key
	log.Debug("Checking Key Pair...", map[string]interface{}{"Name": d.KeyPairName})
	sshKey, _ := client.GetSshkeyByName(d.ProjectID, d.RegionName, d.KeyPairName)
	if sshKey != nil {
		d.KeyPairID = sshKey.ID
		log.Debug("Found key id ", d.KeyPairID)
		return nil
	}

	// Generate key and parent dir if needed
	log.Debug("Creating Key Pair...", map[string]interface{}{"Name": d.KeyPairName})
	keyfile := d.GetSSHKeyPath()
	keypath := filepath.Dir(keyfile)
	err = os.MkdirAll(keypath, 0700)
	if err != nil {
		return err
	}

	err = ssh.GenerateSSHKey(d.GetSSHKeyPath())
	if err != nil {
		return err
	}
	publicKey, err := ioutil.ReadFile(d.publicSSHKeyPath())
	if err != nil {
		return err
	}

	// Upload key
	sshKey, err = client.CreateSshkey(d.ProjectID, d.KeyPairName, string(publicKey))
	if err != nil {
		return err
	}
	d.KeyPairID = sshKey.ID

	log.Debug("Created key id ", d.KeyPairID)
	return nil
}

// waitForInstanceStatus waits until instance reaches status. Copied from openstack Driver
func (d *Driver) waitForInstanceStatus(status string) (instance *Instance, err error) {
	return instance, mcnutils.WaitForSpecificOrError(func() (bool, error) {
		instance, err = d.client.GetInstance(d.ProjectID, d.InstanceID)
		if err != nil {
			return true, err
		}
		log.Debugf("Machine", map[string]interface{}{
			"Name":  d.KeyPairName,
			"State": instance.Status,
		})

		if instance.Status == "ERROR" {
			return true, fmt.Errorf("Instance creation failed. Instance is in ERROR state")
		}

		if instance.Status == status {
			return true, nil
		}

		return false, nil
	}, (statusTimeout / 4), 4*time.Second)
}

// GetSSHHostname returns the hostname for SSH
func (d *Driver) GetSSHHostname() (string, error) {
	return d.IPAddress, nil
}

// GetSSHKeyPath returns the ssh key path
func (d *Driver) GetSSHKeyPath() string {
	return d.SSHKeyPath
}

// Create a new docker machine instance on OVH Cloud
func (d *Driver) Create() error {
	client, err := d.getClient()
	if err != nil {
		return err
	}

	// Ensure ssh key
	err = d.ensureSSHKey()
	if err != nil {
		return err
	}

	// Create instance
	log.Debug("Creating OVH instance...")
	monthlyBilling := d.BillingPeriod == "monthly"
	instance, err := client.CreateInstance(
		d.ProjectID,
		d.MachineName,
		d.KeyPairID,
		d.FlavorID,
		d.ImageID,
		d.RegionName,
		d.NetworkIDs,
		monthlyBilling,
	)
	if err != nil {
		return err
	}
	d.InstanceID = instance.ID

	// Wait until instance is ACTIVE
	log.Debugf("Waiting for OVH instance...", map[string]interface{}{"MachineID": d.InstanceID})
	instance, err = d.waitForInstanceStatus("ACTIVE")
	if err != nil {
		return err
	}

	// Save Ip address
	d.IPAddress = ""
	for _, ip := range instance.IPAddresses {
		if ip.Type == "public" {
			d.IPAddress = ip.IP
			break
		}
	}

	if d.IPAddress == "" {
		return fmt.Errorf("No IP found for instance %s", instance.ID)
	}

	log.Debugf("IP address found", map[string]interface{}{
		"MachineID": d.InstanceID,
		"IP":        d.IPAddress,
	})

	// All done !
	return nil
}

func (d *Driver) publicSSHKeyPath() string {
	return d.GetSSHKeyPath() + ".pub"
}

// GetState return instance status
func (d *Driver) GetState() (state.State, error) {
	log.Debugf("Get status for OVH instance...", map[string]interface{}{"MachineID": d.InstanceID})

	client, err := d.getClient()
	if err != nil {
		return state.None, err
	}

	instance, err := client.GetInstance(d.ProjectID, d.InstanceID)
	if err != nil {
		return state.None, err
	}

	log.Debugf("OVH instance", map[string]interface{}{
		"MachineID": d.InstanceID,
		"State":     instance.Status,
	})

	switch instance.Status {
	case "ACTIVE":
		return state.Running, nil
	case "PAUSED":
		return state.Paused, nil
	case "SUSPENDED":
		return state.Saved, nil
	case "SHUTOFF":
		return state.Stopped, nil
	case "BUILDING":
		return state.Starting, nil
	case "ERROR":
		return state.Error, nil
	}

	return state.None, nil
}

// GetURL returns docker daemon URL on this machine
func (d *Driver) GetURL() (string, error) {
	if d.IPAddress == "" {
		return "", nil
	}
	return fmt.Sprintf("tcp://%s", net.JoinHostPort(d.IPAddress, "2376")), nil
}

// Remove deletes a machine and it's SSH keys from OVH Cloud
func (d *Driver) Remove() error {
	log.Debugf("deleting instance...", map[string]interface{}{"MachineID": d.InstanceID})
	log.Info("Deleting OVH instance...")

	client, err := d.getClient()
	if err != nil {
		return err
	}

	// Deletes instance, if we created it
	if d.InstanceID != "" {
		err = client.DeleteInstance(d.ProjectID, d.InstanceID)
		if err != nil {
			return err
		}
	}

	// If key name  does not starts with the machine ID, this is a pre-existing key, keep it
	if !strings.HasPrefix(d.KeyPairName, d.MachineName) {
		log.Debugf("keeping key pair...", map[string]interface{}{"KeyPairID": d.KeyPairID})
		return nil
	}

	// Deletes ssh key, if we created it
	if d.KeyPairID != "" {
		log.Debugf("deleting key pair...", map[string]interface{}{"KeyPairID": d.KeyPairID})
		err = client.DeleteSshkey(d.ProjectID, d.KeyPairID)
		if err != nil {
			return err
		}
	}

	return nil
}

// Restart this docker-machine
func (d *Driver) Restart() error {
	log.Debugf("Restarting OVH instance...", map[string]interface{}{"MachineID": d.InstanceID})

	client, err := d.getClient()
	if err != nil {
		return err
	}

	err = client.RebootInstance(d.ProjectID, d.InstanceID, false)
	if err != nil {
		return err
	}
	return nil
}

//
// STUBS
//

// Kill (STUB) kill machine
func (d *Driver) Kill() (err error) {
	return fmt.Errorf("Killing machines is not possible on OVH Cloud")
}

// Start (STUB) start machine
func (d *Driver) Start() (err error) {
	return fmt.Errorf("Starting machines is not possible on OVH Cloud")
}

// Stop (STUB) stop machine
func (d *Driver) Stop() (err error) {
	return fmt.Errorf("Stopping machines is not possible on OVH Cloud")
}
