/*
Copyright 2014 The Kubernetes Authors All rights reserved.

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

package unversioned

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/version"
)

// Interface holds the methods for clients of Kubernetes,
// an interface to allow mock testing.
type Interface interface {
	PodsNamespacer
	PodTemplatesNamespacer
	ReplicationControllersNamespacer
	ServicesNamespacer
	EndpointsNamespacer
	VersionInterface
	NodesInterface
	EventNamespacer
	LimitRangesNamespacer
	ResourceQuotasNamespacer
	ServiceAccountsNamespacer
	SecretsNamespacer
	NamespacesInterface
	PersistentVolumesInterface
	PersistentVolumeClaimsNamespacer
	ComponentStatusesInterface
	Extensions() ExtensionsInterface
	ResourcesInterface
}

func (c *Client) ReplicationControllers(namespace string) ReplicationControllerInterface {
	return newReplicationControllers(c, namespace)
}

func (c *Client) Nodes() NodeInterface {
	return newNodes(c)
}

func (c *Client) Events(namespace string) EventInterface {
	return newEvents(c, namespace)
}

func (c *Client) Endpoints(namespace string) EndpointsInterface {
	return newEndpoints(c, namespace)
}

func (c *Client) Pods(namespace string) PodInterface {
	return newPods(c, namespace)
}

func (c *Client) PodTemplates(namespace string) PodTemplateInterface {
	return newPodTemplates(c, namespace)
}

func (c *Client) Services(namespace string) ServiceInterface {
	return newServices(c, namespace)
}
func (c *Client) LimitRanges(namespace string) LimitRangeInterface {
	return newLimitRanges(c, namespace)
}

func (c *Client) ResourceQuotas(namespace string) ResourceQuotaInterface {
	return newResourceQuotas(c, namespace)
}

func (c *Client) ServiceAccounts(namespace string) ServiceAccountsInterface {
	return newServiceAccounts(c, namespace)
}

func (c *Client) Secrets(namespace string) SecretsInterface {
	return newSecrets(c, namespace)
}

func (c *Client) Namespaces() NamespaceInterface {
	return newNamespaces(c)
}

func (c *Client) PersistentVolumes() PersistentVolumeInterface {
	return newPersistentVolumes(c)
}

func (c *Client) PersistentVolumeClaims(namespace string) PersistentVolumeClaimInterface {
	return newPersistentVolumeClaims(c, namespace)
}

func (c *Client) ComponentStatuses() ComponentStatusInterface {
	return newComponentStatuses(c)
}

// VersionInterface has a method to retrieve the server version.
type VersionInterface interface {
	ServerVersion() (*version.Info, error)
	ServerAPIVersions() (*unversioned.APIVersions, error)
}

// ResourcesInterface has methods for obtaining supported resources on the API server
type ResourcesInterface interface {
	SupportedResourcesForGroupVersion(groupVersion string) (*unversioned.APIResourceList, error)
}

// APIStatus is exposed by errors that can be converted to an api.Status object
// for finer grained details.
type APIStatus interface {
	Status() unversioned.Status
}

// Client is the implementation of a Kubernetes client.
type Client struct {
	*RESTClient
	*ExtensionsClient
}

// ServerVersion retrieves and parses the server's version.
func (c *Client) ServerVersion() (*version.Info, error) {
	body, err := c.Get().AbsPath("/version").Do().Raw()
	if err != nil {
		return nil, err
	}
	var info version.Info
	err = json.Unmarshal(body, &info)
	if err != nil {
		return nil, fmt.Errorf("got '%s': %v", string(body), err)
	}
	return &info, nil
}

// SupportedResourcesForGroupVersion retrieves the list of resources supported by the API server for a group version.
func (c *Client) SupportedResourcesForGroupVersion(groupVersion string) (*unversioned.APIResourceList, error) {
	var prefix string
	if groupVersion == "v1" {
		prefix = "/api"
	} else {
		prefix = "/apis"
	}
	body, err := c.Get().AbsPath(prefix, groupVersion).Do().Raw()
	if err != nil {
		return nil, err
	}
	resources := unversioned.APIResourceList{}
	if err := json.Unmarshal(body, &resources); err != nil {
		return nil, err
	}
	return &resources, nil
}

// SupportedResources gets all supported resources for all group versions.  The key in the map is an API groupVersion.
func SupportedResources(c Interface, cfg *Config) (map[string]*unversioned.APIResourceList, error) {
	apis, err := ServerAPIVersions(cfg)
	if err != nil {
		return nil, err
	}
	result := map[string]*unversioned.APIResourceList{}
	for _, groupVersion := range apis {
		resources, err := c.SupportedResourcesForGroupVersion(groupVersion)
		if err != nil {
			return nil, err
		}
		result[groupVersion] = resources
	}
	return result, nil
}

// ServerAPIVersions retrieves and parses the list of API versions the server supports.
func (c *Client) ServerAPIVersions() (*unversioned.APIVersions, error) {
	body, err := c.Get().UnversionedPath("").Do().Raw()
	if err != nil {
		return nil, err
	}
	var v unversioned.APIVersions
	err = json.Unmarshal(body, &v)
	if err != nil {
		return nil, fmt.Errorf("got '%s': %v", string(body), err)
	}
	return &v, nil
}

type ComponentValidatorInterface interface {
	ValidateComponents() (*api.ComponentStatusList, error)
}

// ValidateComponents retrieves and parses the master's self-monitored cluster state.
// TODO: This should hit the versioned endpoint when that is implemented.
func (c *Client) ValidateComponents() (*api.ComponentStatusList, error) {
	body, err := c.Get().AbsPath("/validate").DoRaw()
	if err != nil {
		return nil, err
	}

	statuses := []api.ComponentStatus{}
	if err := json.Unmarshal(body, &statuses); err != nil {
		return nil, fmt.Errorf("got '%s': %v", string(body), err)
	}
	return &api.ComponentStatusList{Items: statuses}, nil
}

// IsTimeout tests if this is a timeout error in the underlying transport.
// This is unbelievably ugly.
// See: http://stackoverflow.com/questions/23494950/specifically-check-for-timeout-error for details
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	switch err := err.(type) {
	case *url.Error:
		if err, ok := err.Err.(net.Error); ok {
			return err.Timeout()
		}
	case net.Error:
		return err.Timeout()
	}

	if strings.Contains(err.Error(), "use of closed network connection") {
		return true
	}
	return false
}

func (c *Client) Extensions() ExtensionsInterface {
	return c.ExtensionsClient
}
