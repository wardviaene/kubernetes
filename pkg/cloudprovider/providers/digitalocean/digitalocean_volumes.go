/*
Copyright 2016 The Kubernetes Authors.

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
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/volume"
)


// Create a volume of given size (in GiB)
func (do *DigitalOcean) CreateVolume(region string, name string, description string, sizeGigaBytes int64) (volumeName string, err error) {
	volumeCreateRequest := godo.VolumeCreateRequest{
		Region: region,
		Name: name,
		Description: description,
		SizeGigaBytes: sizeGigaBytes,
	}
	vol, _, err := do.provider.Storage.CreateVolume(&volumeCreateRequest)
	if err != nil {
		glog.Errorf("Failed to create a %d GB volume: %v", volumeCreateRequest.SizeGigaBytes, err)
		return "", err
	}
	glog.Infof("Created volume %v", vol.ID)
	return vol.ID, err
}

// Delete a volume
func (do *DigitalOcean) DeleteVolume(volumeID string) error {
	used, err := do.volumeIsUsed(volumeID)
	if err != nil {
		return err
	}
	if used {
		msg := fmt.Sprintf("Cannot delete the volume %s, it's still attached to a node", volumeID)
		return volume.NewDeletedVolumeInUseError(msg)
	}

	_, err = do.provider.Storage.DeleteVolume(volumeID)
	if err != nil {
		glog.Errorf("Cannot delete volume %s: %v", volumeID, err)
	}
	return err
}

// volumeIsUsed returns true if a volume is attached to a node.
func (do *DigitalOcean) volumeIsUsed(volumeID string) (bool, error) {
	volume, _, err := do.provider.Storage.GetVolume(volumeID)
	if err != nil {
		return false, err
	}
	if len(volume.DropletIDs) > 0 {
		return true, nil
	}
	return false, nil
}

// Attaches given DigitalOcean volume
func (do *DigitalOcean) AttachVolume(instanceID int, volumeID string) (string, error) {
	_, _, err := do.provider.StorageActions.Attach(volumeID, instanceID)
	if err != nil {
		return "", err
	}

	if err != nil {
		glog.Errorf("Failed to attach %s volume to %s compute", volumeID, instanceID)
		return "", err
	}
	glog.V(2).Infof("Successfully attached %s volume to %s compute", volumeID, instanceID)
	return volumeID, nil
}

// Detaches given cinder volume from the compute running kubelet
func (do *DigitalOcean) DetachVolume(instanceID int, volumeID string) error {
	_, _, err := do.provider.StorageActions.Detach(volumeID)
	if err != nil {
		glog.Errorf("Failed to detach %s volume", volumeID)
		return err
	}
	glog.V(2).Infof("Successfully detached %s volume", volumeID)
	return nil
}

func (do *DigitalOcean) getVolume(volumeID string) (*godo.Volume, error) {
	volume, _, err := do.provider.Storage.GetVolume(volumeID)
	if err != nil {
		glog.Errorf("Error occurred getting volume: %s", volumeID)
		return volume, err
	}
	return volume, err
}


// GetDevicePath returns the path of an attached block storage volume, specified by its id.
func (do *DigitalOcean) GetDevicePath(volumeId string) string {
	files, _ := ioutil.ReadDir("/dev/disk/by-id/")
	for _, f := range files {
		if strings.Contains(f.Name(), "scsi-0DO_Volume_") {
			devid_prefix := f.Name()[len("scsi-0DO_Volume_"):len(f.Name())]
			if strings.Contains(volumeId, devid_prefix) {
				glog.V(4).Infof("Found disk attached as %q; full devicepath: %s\n", f.Name(), path.Join("/dev/disk/by-id/", f.Name()))
				return path.Join("/dev/disk/by-id/", f.Name())
			}
		}
	}
	glog.Warningf("Failed to find device for the diskid: %q\n", volumeId)
	return ""
}

// Get device path of attached volume to the compute running kubelet
func (do *DigitalOcean) GetAttachmentVolumePath(instanceID int, volumeID string) (string, error) {
	volume, err := do.getVolume(volumeID)
	if err != nil {
		return "", err
	}
	if len(volume.DropletIDs) == 0 {
		return "", fmt.Errorf("volume %s is not attached to %d", volumeID, instanceID)
	}
	attached := false
	for _, i := range volume.DropletIDs {
		if(i == instanceID) {
			attached = true
		}
  }
	if(!attached) {
		return "", fmt.Errorf("volume %s is not attached to %d", volumeID, instanceID)
	}
	return "/dev/disk/by-id/scsi-0DO_Volume_"+volume.Name, nil
}

// query if a volume is attached to a compute instance
func (do *DigitalOcean) DiskIsAttached(volumeID string, instanceID int) (bool, error) {
	volume, err := do.getVolume(volumeID)
	if err != nil {
		return false, err
	}
	if len(volume.DropletIDs) == 0 {
		return false, nil
	}
	attached := false
	for _, i := range volume.DropletIDs {
		if(i == instanceID) {
			attached = true
		}
  }
	return attached, nil
}

// query if a list of volumes are attached to a compute instance
func (do *DigitalOcean) DisksAreAttached(volumeIDs []string, instanceID int) (map[string]bool, error) {
	attached := make(map[string]bool)
	for _, volumeID := range volumeIDs {
		attached[volumeID] = false
	}
	for _, volumeID := range volumeIDs {
		volume, err := do.getVolume(volumeID)
		if err != nil {
			continue
		}
		for _, i := range volume.DropletIDs {
			if(i == instanceID) {
				attached[volumeID] = true
			}
		}
	}
	return attached, nil
}

