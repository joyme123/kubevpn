package dev

import (
	"github.com/containerd/containerd/platforms"
	"github.com/docker/docker/api/types/network"
	"github.com/pkg/errors"

	"github.com/wencaiwulue/kubevpn/pkg/util"
)

func mergeDockerOptions(r ConfigList, copts *Options, tempContainerConfig *containerConfig) error {
	if copts.ContainerName != "" {
		var index = -1
		for i, config := range r {
			if config.k8sContainerName == copts.ContainerName {
				index = i
				break
			}
		}
		if index != -1 {
			r[0], r[index] = r[index], r[0]
		}
	}

	config := r[0]
	config.Options = copts.Options
	config.Copts = copts.Copts

	if copts.DockerImage != "" {
		config.config.Image = copts.DockerImage
	}
	if copts.Options.Name != "" {
		config.containerName = copts.Options.Name
	} else {
		config.Options.Name = config.containerName
	}
	if copts.Options.Platform != "" {
		p, err := platforms.Parse(copts.Options.Platform)
		if err != nil {
			return errors.Wrap(err, "error parsing specified platform")
		}
		config.platform = &p
	}

	tempContainerConfig.HostConfig.CapAdd = append(tempContainerConfig.HostConfig.CapAdd, config.hostConfig.CapAdd...)
	tempContainerConfig.HostConfig.SecurityOpt = append(tempContainerConfig.HostConfig.SecurityOpt, config.hostConfig.SecurityOpt...)
	tempContainerConfig.HostConfig.VolumesFrom = append(tempContainerConfig.HostConfig.VolumesFrom, config.hostConfig.VolumesFrom...)
	tempContainerConfig.HostConfig.DNS = append(tempContainerConfig.HostConfig.DNS, config.hostConfig.DNS...)
	tempContainerConfig.HostConfig.DNSOptions = append(tempContainerConfig.HostConfig.DNSOptions, config.hostConfig.DNSOptions...)
	tempContainerConfig.HostConfig.DNSSearch = append(tempContainerConfig.HostConfig.DNSSearch, config.hostConfig.DNSSearch...)

	config.hostConfig = tempContainerConfig.HostConfig
	config.networkingConfig.EndpointsConfig = util.Merge[string, *network.EndpointSettings](tempContainerConfig.NetworkingConfig.EndpointsConfig, config.networkingConfig.EndpointsConfig)

	c := tempContainerConfig.Config
	var entrypoint = config.config.Entrypoint
	var args = config.config.Cmd
	// if special --entrypoint, then use it
	if len(c.Entrypoint) != 0 {
		entrypoint = c.Entrypoint
		args = c.Cmd
	}
	if len(c.Cmd) != 0 {
		args = c.Cmd
	}
	c.Entrypoint = entrypoint
	c.Cmd = args
	c.Env = append(config.config.Env, c.Env...)
	c.Image = config.config.Image
	if c.User == "" {
		c.User = config.config.User
	}
	c.Labels = util.Merge[string, string](config.config.Labels, c.Labels)
	c.Volumes = util.Merge[string, struct{}](c.Volumes, config.config.Volumes)
	if c.WorkingDir == "" {
		c.WorkingDir = config.config.WorkingDir
	}

	config.config = c

	return nil
}
