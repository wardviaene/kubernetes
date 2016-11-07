/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package digitalocean

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"strconv"

	"gopkg.in/gcfg.v1"

	"github.com/digitalocean/godo"
  "golang.org/x/oauth2"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/cloudprovider"
  "k8s.io/kubernetes/pkg/types"
  "k8s.io/kubernetes/pkg/api"
)

const ProviderName = "digitalocean"

var ErrNotFound = errors.New("Failed to find object")
var ErrMultipleResults = errors.New("Multiple results where only one expected")
var ErrNoAddressFound = errors.New("No address found for host")
var ErrAttrNotFound = errors.New("Expected attribute not found")

type DigitalOcean struct {
	provider *godo.Client
	region   string
	selfDOInstance *doInstance
}

type doInstance struct {
	// Local droplet ID
  dropletID int

  // region the instance resides in
  region string
}


type Config struct {
	Global struct {
		ApiKey     string `gcfg:"apikey"`
		Region     string `gcfg:"region"`
	}
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		cfg, err := readConfig(config)
		if err != nil {
			return nil, err
		}
		return newDigitalOcean(cfg)
	})
}


func readConfig(config io.Reader) (Config, error) {
	if config == nil {
		err := fmt.Errorf("no DigitalOcean cloud provider config file given. Restart process with --cloud-provider=digitalocean --cloud-config=[path_to_config_file]")
		return Config{}, err
	}

	var cfg Config
	err := gcfg.ReadInto(&cfg, config)
	return cfg, err
}

func (t *TokenSource) Token() (*oauth2.Token, error) {
    token := &oauth2.Token{
        AccessToken: t.AccessToken,
    }
    return token, nil
}
type TokenSource struct {
    AccessToken string
}


func newDigitalOcean(cfg Config) (*DigitalOcean, error) {
  tokenSource := &TokenSource{
      AccessToken: cfg.Global.ApiKey,
  }
  oauthClient := oauth2.NewClient(oauth2.NoContext, tokenSource)
  provider := godo.NewClient(oauthClient)

  _, _, err := provider.Account.Get()
  if err != nil {
		return nil, err
  }
	do := DigitalOcean{
		provider: provider,
		region:   cfg.Global.Region,
	}

	// build self DigitalOcean Instance information
  selfDOInstance, err := do.buildSelfDOInstance()
  if err != nil {
    return nil, err
  }
	glog.V(2).Infof("DigitalOcean Droplet region: %s, droplet ID: %d", selfDOInstance.region, selfDOInstance.dropletID)

	return &do, nil
}

func (do *DigitalOcean) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// implementation of interfaces
func (do *DigitalOcean) Instances() (cloudprovider.Instances, bool) {
	return do, true
}
func (do *DigitalOcean) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return nil, false
}
func (do *DigitalOcean) Zones() (cloudprovider.Zones, bool) {
	return do, false
}
func (do *DigitalOcean) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}
// ScrubDNS filters DNS settings for pods.
func (do *DigitalOcean) ScrubDNS(nameservers, searches []string) (nsOut, srchOut []string) {
	return nameservers, searches
}
// ProviderName returns the cloud provider ID.
func (do *DigitalOcean) ProviderName() string {
	return ProviderName
}
// helper func
func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
func (do *DigitalOcean) findDroplet(name types.NodeName) (*godo.Droplet, error) {
	listOptions := &godo.ListOptions{
		Page: 1,
		PerPage: 200,
	}
  droplets, _, err := do.provider.Droplets.List(listOptions)
  if err != nil {
		return nil, err
  }
	for i := 0; i < len(droplets); i++ {
		if strings.ToLower(string(name)) == strings.ToLower(droplets[i].Name) {
			return &droplets[i], nil
		}
		ipv4, err := droplets[i].PrivateIPv4()
		if err == nil && string(name) == ipv4 {
			return &droplets[i], nil
		}
		ipv4, err = droplets[i].PublicIPv4()
		if err == nil && string(name) == ipv4 {
			return &droplets[i], nil
		}
	}
	return nil, ErrNotFound
}
func (do *DigitalOcean) findDropletByFilter(filter string) ([]types.NodeName, error) {
	list := []types.NodeName{}
	listOptions := &godo.ListOptions{
		Page: 1,
		PerPage: 200,
	}
  droplets, _, err := do.provider.Droplets.List(listOptions)
  if err != nil {
		return nil, err
  }
	for i := 0; i < len(droplets); i++ {
		if(strings.Contains(droplets[i].Name, filter)) {
			list = append(list, types.NodeName(droplets[i].Name))
		}
	}
	return list, nil
}
func (do *DigitalOcean) NodeAddresses(name types.NodeName) ([]api.NodeAddress, error) {
	addresses := []api.NodeAddress{}
	droplet, err := do.findDroplet(name);
  if err != nil {
		return nil, err
  }
	internalIP, err := droplet.PrivateIPv4()
  if err != nil {
		return nil, err
  }
	externalIP, err := droplet.PublicIPv4()
  if err != nil {
		return nil, err
  }
	addresses = append(addresses, api.NodeAddress{Type: api.NodeInternalIP, Address: internalIP})
	// Legacy compatibility: the private ip was the legacy host ip
	addresses = append(addresses, api.NodeAddress{Type: api.NodeLegacyHostIP, Address: internalIP})
	addresses = append(addresses, api.NodeAddress{Type: api.NodeExternalIP, Address: externalIP})
  return addresses, nil
}
func (do *DigitalOcean) ExternalID(nodeName types.NodeName) (string, error) {
	droplet, err := do.findDroplet(nodeName);
  if err != nil {
		return "", cloudprovider.InstanceNotFound
  } else {
		return strconv.Itoa(droplet.ID), nil
	}
}

func (do *DigitalOcean) InstanceID(nodeName types.NodeName) (string, error) {
	droplet, err := do.findDroplet(nodeName);
  if err != nil {
		return "", cloudprovider.InstanceNotFound
  } else {
		return strconv.Itoa(droplet.ID), nil
	}
}
func (do *DigitalOcean) LocalInstanceID() (string, error) {
  return strconv.Itoa(do.selfDOInstance.dropletID), nil
}

func (do *DigitalOcean) InstanceType(nodeName types.NodeName) (string, error) {
	droplet, err := do.findDroplet(nodeName);
  if err != nil {
		return "", cloudprovider.InstanceNotFound
  } else {
		return droplet.Size.Slug, nil
	}
}
func (do *DigitalOcean) List(filter string) ([]types.NodeName, error) {
	list, err := do.findDropletByFilter(filter);
  if err != nil {
		return nil, cloudprovider.InstanceNotFound
  } else {
		return list, nil
	}
}
// AddSSHKeyToAllInstances is currently unimplemented
func (do *DigitalOcean) AddSSHKeyToAllInstances(user string, keyData []byte) error {
  return errors.New("unimplemented")
}
func (do *DigitalOcean) CurrentNodeName(hostname string) (types.NodeName, error) {
	droplet, err := do.findDroplet(types.NodeName(hostname));
  if err != nil {
		return "", cloudprovider.InstanceNotFound
  } else {
		return types.NodeName(strings.ToLower(droplet.Name)), nil
	}
}

func (do *DigitalOcean) GetRegion() string {
	return do.region
}

// Zones
func (do *DigitalOcean) GetZone() (cloudprovider.Zone, error) {
	return cloudprovider.Zone{
		FailureDomain: do.selfDOInstance.region,
		Region:        do.selfDOInstance.region,
	}, nil
}

// metadata
func (do *DigitalOcean) buildSelfDOInstance() (*doInstance, error) {
  if do.selfDOInstance != nil {
    panic("do not call buildSelfDOInstance directly")
  }

	// get region
	resp, err := http.Get("http://169.254.169.254/metadata/v1/region")
	if err != nil {
		glog.V(2).Infof("error fetching region from metadata service: %v", err)
    return nil, err
	}
	defer resp.Body.Close()
	dropletRegion, err := ioutil.ReadAll(resp.Body)

	// get droplet id
	resp, err = http.Get("http://169.254.169.254/metadata/v1/id")
	if err != nil {
		glog.V(2).Infof("error fetching droplet id from metadata service: %v", err)
    return nil, err
	}
	defer resp.Body.Close()
	dropletID, err := ioutil.ReadAll(resp.Body)
	intDropletID, err := strconv.Atoi(string(dropletID))
	if err != nil {
		glog.V(2).Infof("dropletID is invalid")
    return nil, err
	}

	self := &doInstance{
		dropletID: intDropletID,
		region: string(dropletRegion),
	}
  return self, nil
}

