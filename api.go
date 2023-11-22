package main

import (
	"fmt"
	"strings"

	"github.com/ovh/go-ovh/ovh"
)

const (
	// CustomerInterface is the URL of the customer interface, for error messages
	CustomerInterface = "https://www.ovh.com/manager/cloud/index.html"
)

// API is a handle to an instanciated OVH API.
type API struct {
	client *ovh.Client
}

// Project is a go representation of a Cloud project
type Project struct {
	Name         string `json:"description"`
	ID           string `json:"project_id"`
	Unleash      bool   `json:"unleash"`
	CreationDate string `json:"creationDate"`
	OrderID      int    `json:"orderID"`
	Status       string `json:"status"`
}

// Projects is a list of project IDs
type Projects []string

// Flavor is a go representation of Cloud Flavor
type Flavor struct {
	Region      string `json:"region"`
	Name        string `json:"name"`
	ID          string `json:"id"`
	OS          string `json:"osType"`
	Vcpus       int    `json:"vcpus"`
	MemoryGB    int    `json:"ram"`
	DiskSpaceGB int    `json:"disk"`
	Type        string `json:"type"`
}

// Flavors is a list flavors
type Flavors []Flavor

// Image is a go representation of a Cloud Image (VM template)
type Image struct {
	Region       string `json:"region"`
	Name         string `json:"name"`
	ID           string `json:"id"`
	OS           string `json:"type"`
	CreationDate string `json:"creationDate"`
	Status       string `json:"status"`
	MinDisk      int    `json:"minDisk"`
	Visibility   string `json:"visibility"`
}

// Images is a list of Images
type Images []Image

// Regions is a list of Cloud Region names
type Regions []string

// Network defines the private network names
type Network struct {
	Status string `json:"status"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	ID     string `json:"id"`
	VlanID int    `json:"vlanid"`
}

// Networks is a list of Network
type Networks []Network

// SshkeyReq defines the fields for an SSH Key upload
type SshkeyReq struct {
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	Region    string `json:"region,omitempty"`
}

// Sshkey is a go representation of Cloud SSH Key
type Sshkey struct {
	Name        string  `json:"name"`
	ID          string  `json:"id"`
	PublicKey   string  `json:"publicKey"`
	Fingerprint string  `json:"fingerPrint"`
	Regions     Regions `json:"region"`
}

// Sshkeys is a list of Sshkey
type Sshkeys []Sshkey

// IP is a go representation of a Cloud IP address
type IP struct {
	IP   string `json:"ip"`
	Type string `json:"type"`
}

// IPs is a list of IPs
type IPs []IP

// NetworkParmas for Cloud instance
type NetworkParam struct {
	ID string `json:"networkId"`
}

type NetworkParams []NetworkParam

// InstanceReq defines the fields for a VM creation
type InstanceReq struct {
	Name           string        `json:"name"`
	FlavorID       string        `json:"flavorName"`
	ImageID        string        `json:"imageID"`
	Region         string        `json:"region"`
	NetworkParams  NetworkParams `json:"networks"`
	SshkeyID       string        `json:"sshKeyID"`
	MonthlyBilling bool          `json:"monthlyBilling"`
}

// Instance is a go representation of Cloud instance
type Instance struct {
	Name           string        `json:"name"`
	ID             string        `json:"id"`
	Status         string        `json:"status"`
	Created        string        `json:"created"`
	Region         string        `json:"region"`
	NetworkParams  NetworkParams `json:"networks"`
	Image          Image         `json:"image"`
	Flavor         Flavor        `json:"flavor"`
	Sshkey         Sshkey        `json:"sshKey"`
	IPAddresses    IPs           `json:"ipAddresses"`
	MonthlyBilling bool          `json:"monthlyBilling"`
}

// RebootReq defines the fields for a VM reboot
type RebootReq struct {
	Type string `json:"type"`
}

// NewAPI instanciates a Cloud API driver from credentials, for a given endpoint. See github.com/ovh/go-ovh for more informations
func NewAPI(endpoint, applicationKey, applicationSecret, consumerKey string) (api *API, err error) {
	client, err := ovh.NewClient(endpoint, applicationKey, applicationSecret, consumerKey)
	return &API{client}, err
}

// GetProjects returns a list of string project ID
func (a *API) GetProjects() (projects Projects, err error) {
	err = a.client.Get("/cloud/project", &projects)
	return projects, err
}

// GetProject return the details of a project given a project id
func (a *API) GetProject(projectID string) (project *Project, err error) {
	err = a.client.Get("/cloud/project/"+projectID, &project)
	return project, err
}

// GetProjectByName returns the details of a project given its name. This is slower than GetProject
func (a *API) GetProjectByName(projectName string) (project *Project, err error) {
	// get project list
	projects, err := a.GetProjects()
	if err != nil {
		return nil, err
	}

	// If projectName is a valid projectID return it.
	for _, projectID := range projects {
		if projectID == projectName {
			return a.GetProject(projectID)
		}
	}

	// Attempt to find a project matching projectName. This is potentially slow
	for _, projectID := range projects {
		project, err := a.GetProject(projectID)
		if err != nil {
			return nil, err
		}

		if project.Name == projectName {
			return project, nil
		}
	}

	// Ooops
	return nil, fmt.Errorf("Project '%s' does not exist on OVH cloud. To create or rename a project, please visit %s", projectName, CustomerInterface)
}

// GetNetworks returns public & private networks for a given project
func (a *API) GetNetworks(projectID string, privateNet bool) (networks Networks, err error) {
	// if network type is true lets get the private network
	var url string
	if privateNet == true {
		url = fmt.Sprintf("/cloud/project/%s/network/private", projectID)
	} else {
		url = fmt.Sprintf("/cloud/project/%s/network/public", projectID)
	}
	err = a.client.Get(url, &networks)
	return networks, err
}

// GetPublicNetworkID returns the public network id for a given project
func (a *API) GetPublicNetworkID(projectID string) (publicID string, err error) {
	networks, err := a.GetNetworks(projectID, false)
	if err != nil {
		return "", err
	}
	return networks[0].ID, nil
}

// GetNetworksByName returns the details of a network given its name & project
func (a *API) GetPrivateNetworkByName(projectID, networkName string) (network *Network, err error) {
	// Get image list
	networks, err := a.GetNetworks(projectID, true)
	if err != nil {
		return nil, err
	}

	// Find first matching network
	for _, network := range networks {
		if fmt.Sprintf("%d", network.VlanID) == networkName || network.Name == networkName {
			return &network, nil
		}
	}

	var networkNames []string
	for _, network := range networks {
		networkNames = append(networkNames, network.Name)
	}

	return nil, fmt.Errorf("Invalid private network %s. List of valid private networks include %s", networkName, strings.Join(networkNames[:], ", "))
}

// GetRegions returns the list of valid regions for a given project
func (a *API) GetRegions(projectID string) (regions Regions, err error) {
	url := fmt.Sprintf("/cloud/project/%s/region", projectID)
	err = a.client.Get(url, &regions)
	return regions, err
}

// GetFlavors returns the list of available flavors for a given project in a giver zone
func (a *API) GetFlavors(projectID, region string) (flavors Flavors, err error) {
	url := fmt.Sprintf("/cloud/project/%s/flavor?region=%s", projectID, region)
	err = a.client.Get(url, &flavors)
	return flavors, err
}

// GetFlavorByName returns the details of a flavor given its name. Slower than getting by id
func (a *API) GetFlavorByName(projectID, region, flavorName string) (flavor *Flavor, err error) {
	// Get flavor list
	flavors, err := a.GetFlavors(projectID, region)
	if err != nil {
		return nil, err
	}

	// Find first matching Linux flavor
	for _, flavor := range flavors {
		if flavor.OS != "linux" {
			continue
		}

		if flavor.ID == flavorName || flavor.Name == flavorName {
			return &flavor, nil
		}
	}

	// Ooops
	return nil, fmt.Errorf("Flavor '%s' does not exist on OVH cloud. To find a list of available flavors, please visit %s", flavorName, CustomerInterface)
}

// GetImages returns a list of images for a given project in a given region
func (a *API) GetImages(projectID, region string) (images Images, err error) {
	url := fmt.Sprintf("/cloud/project/%s/image?osType=linux&region=%s", projectID, region)
	err = a.client.Get(url, &images)
	return images, err
}

// GetImageByName returns the details of an image given its name, a project and a region. This is slower than id access
func (a *API) GetImageByName(projectID, region, imageName string) (image *Image, err error) {
	// Get image list
	images, err := a.GetImages(projectID, region)
	if err != nil {
		return nil, err
	}

	// Find first matching image
	for _, image := range images {
		if image.OS != "linux" {
			continue
		}

		if image.ID == imageName || image.Name == imageName {
			return &image, nil
		}
	}

	// Ooops
	return nil, fmt.Errorf("Image '%s' does not exist on OVH cloud. To find a list of available images, please visit %s", imageName, CustomerInterface)
}

// GetSshkeys returns a list of sshkeys for a given project in a given region
func (a *API) GetSshkeys(projectID, region string) (sshkeys Sshkeys, err error) {
	url := fmt.Sprintf("/cloud/project/%s/sshkey?region=%s", projectID, region)
	err = a.client.Get(url, &sshkeys)
	return sshkeys, err
}

// GetSshkeyByName returns the details of an ssh key given its name in a given region. This is slower than id access
func (a *API) GetSshkeyByName(projectID, region, sshKeyName string) (sshkey *Sshkey, err error) {
	// Get sshkey list
	sshkeys, err := a.GetSshkeys(projectID, region)
	if err != nil {
		return nil, err
	}

	// Find first matching sshkey
	for _, sshkey := range sshkeys {
		if sshkey.ID == sshKeyName || sshkey.Name == sshKeyName {
			return &sshkey, nil
		}
	}

	// Ooops
	return nil, fmt.Errorf("SSH key '%s' does not exist on OVH cloud. To find a list of available ssh keys, please visit %s", sshKeyName, CustomerInterface)
}

// CreateSshkey uploads a new public key with name and returns resulting object
func (a *API) CreateSshkey(projectID, name, pubkey string) (sshkey *Sshkey, err error) {
	var sshkeyreq SshkeyReq
	sshkeyreq.Name = name
	sshkeyreq.PublicKey = pubkey

	url := fmt.Sprintf("/cloud/project/%s/sshkey", projectID)
	err = a.client.Post(url, sshkeyreq, &sshkey)
	return sshkey, err
}

// DeleteSshkey deletes an existing sshkey
func (a *API) DeleteSshkey(projectID, instanceID string) (err error) {
	url := fmt.Sprintf("/cloud/project/%s/sshkey/%s", projectID, instanceID)
	err = a.client.Delete(url, nil)
	if apierror, ok := err.(*ovh.APIError); ok && apierror.Code == 404 {
		err = nil
	}
	return err
}

// CreateInstance start a new public cloud instance and returns resulting object
func (a *API) CreateInstance(projectID, name, pubkeyID, flavorID, ImageID, region string, networkIDs []string, monthlyBilling bool) (instance *Instance, err error) {
	var instanceReq InstanceReq
	instanceReq.Name = name
	instanceReq.SshkeyID = pubkeyID
	instanceReq.FlavorID = flavorID
	instanceReq.ImageID = ImageID
	instanceReq.Region = region
	instanceReq.MonthlyBilling = monthlyBilling

	for _, v := range networkIDs {
		networkParam := NetworkParam{ID: v}
		instanceReq.NetworkParams = append(instanceReq.NetworkParams, networkParam)
	}

	url := fmt.Sprintf("/cloud/project/%s/instance", projectID)
	err = a.client.Post(url, instanceReq, &instance)
	return instance, err
}

// RebootInstance reboot an instance
func (a *API) RebootInstance(projectID, instanceID string, hard bool) (err error) {
	var rebootReq RebootReq
	if hard == true {
		rebootReq.Type = "hard"
	} else {
		rebootReq.Type = "soft"
	}

	url := fmt.Sprintf("/cloud/project/%s/instance/%s/reboot", projectID, instanceID)
	err = a.client.Post(url, rebootReq, nil)
	return err
}

// DeleteInstance stops and destroys a public cloud instance
func (a *API) DeleteInstance(projectID, instanceID string) (err error) {
	url := fmt.Sprintf("/cloud/project/%s/instance/%s", projectID, instanceID)
	err = a.client.Delete(url, nil)
	if apierror, ok := err.(*ovh.APIError); ok && apierror.Code == 404 {
		err = nil
	}
	return err
}

// GetInstance finds a VM instance given a name or an ID
func (a *API) GetInstance(projectID, instanceID string) (instance *Instance, err error) {
	url := fmt.Sprintf("/cloud/project/%s/instance/%s", projectID, instanceID)
	err = a.client.Get(url, &instance)
	return instance, nil
}
