// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package command

import (
	"fmt"
	"sort"
	"strings"

	humanize "github.com/dustin/go-humanize"
	"github.com/hashicorp/nomad/api"
)

func (c *VolumeStatusCommand) hostVolumeStatus(client *api.Client, id, nodeID, nodePool string) int {
	if id == "" {
		return c.listHostVolumes(client, nodeID, nodePool)
	}

	if nodeID != "" || nodePool != "" {
		c.Ui.Error("-node or -node-pool options can only be used when no ID is provided")
		return 1
	}

	// get a host volume that matches the given prefix or a list of all matches
	// if an exact match is not found. note we can't use the shared getByPrefix
	// helper here because the List API doesn't match the required signature

	volStub, possible, err := c.getByPrefix(client, id)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error listing volumes: %s", err))
		return 1
	}
	if len(possible) > 0 {
		out, err := c.formatHostVolumes(possible)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error formatting: %s", err))
			return 1
		}
		c.Ui.Error(fmt.Sprintf("Prefix matched multiple volumes\n\n%s", out))
		return 1
	}

	vol, _, err := client.HostVolumes().Get(volStub.ID, nil)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error querying volume: %s", err))
		return 1
	}

	str, err := c.formatHostVolume(vol)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error formatting volume: %s", err))
		return 1
	}
	c.Ui.Output(str)
	return 0
}

func (c *VolumeStatusCommand) listHostVolumes(client *api.Client, nodeID, nodePool string) int {
	vols, _, err := client.HostVolumes().List(&api.HostVolumeListRequest{
		NodeID:   nodeID,
		NodePool: nodePool,
	}, nil)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error querying volumes: %s", err))
		return 1
	}

	str, err := c.formatHostVolumes(vols)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error formatting volumes: %s", err))
		return 1
	}
	c.Ui.Output(str)

	return 0
}

func (c *VolumeStatusCommand) getByPrefix(client *api.Client, prefix string) (*api.HostVolumeStub, []*api.HostVolumeStub, error) {
	vols, _, err := client.HostVolumes().List(nil, &api.QueryOptions{
		Prefix:    prefix,
		Namespace: c.namespace,
	})

	if err != nil {
		return nil, nil, fmt.Errorf("error querying volumes: %s", err)
	}
	switch len(vols) {
	case 0:
		return nil, nil, fmt.Errorf("no volumes with prefix or ID %q found", prefix)
	case 1:
		return vols[0], nil, nil
	default:
		// search for exact matches to account for multiple exact ID or name
		// matches across namespaces
		var match *api.HostVolumeStub
		exactMatchesCount := 0
		for _, vol := range vols {
			if vol.ID == prefix || vol.Name == prefix {
				exactMatchesCount++
				match = vol
			}
		}
		if exactMatchesCount == 1 {
			return match, nil, nil
		}
		return nil, vols, nil
	}
}

func (c *VolumeStatusCommand) formatHostVolume(vol *api.HostVolume) (string, error) {
	if c.json || len(c.template) > 0 {
		out, err := Format(c.json, c.template, vol)
		if err != nil {
			return "", fmt.Errorf("format error: %v", err)
		}
		return out, nil
	}

	output := []string{
		fmt.Sprintf("ID|%s", vol.ID),
		fmt.Sprintf("Name|%s", vol.Name),
		fmt.Sprintf("Namespace|%s", vol.Namespace),
		fmt.Sprintf("Plugin ID|%s", vol.PluginID),
		fmt.Sprintf("Node ID|%s", vol.NodeID),
		fmt.Sprintf("Node Pool|%s", vol.NodePool),
		fmt.Sprintf("Capacity|%s", humanize.IBytes(uint64(vol.CapacityBytes))),
		fmt.Sprintf("State|%s", vol.State),
		fmt.Sprintf("Host Path|%s", vol.HostPath),
	}

	// Exit early
	if c.short {
		return formatKV(output), nil
	}

	full := []string{formatKV(output)}

	// Format the allocs
	banner := c.Colorize().Color("\n[bold]Allocations[reset]")
	allocs := formatAllocListStubs(vol.Allocations, c.verbose, c.length)
	full = append(full, banner)
	full = append(full, allocs)

	return strings.Join(full, "\n"), nil
}

func (c *VolumeStatusCommand) formatHostVolumes(vols []*api.HostVolumeStub) (string, error) {
	// Sort the output by volume ID
	sort.Slice(vols, func(i, j int) bool { return vols[i].ID < vols[j].ID })

	if c.json || len(c.template) > 0 {
		out, err := Format(c.json, c.template, vols)
		if err != nil {
			return "", fmt.Errorf("format error: %v", err)
		}
		return out, nil
	}

	// Truncate the id unless full length is requested
	length := shortId
	if c.verbose {
		length = fullId
	}

	rows := make([]string, len(vols)+1)
	rows[0] = "ID|Name|Namespace|Plugin ID|Node ID|Node Pool|State"
	for i, v := range vols {
		rows[i+1] = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
			limit(v.ID, length),
			v.Name,
			v.Namespace,
			v.PluginID,
			limit(v.NodeID, length),
			v.NodePool,
			v.State,
		)
	}
	return formatList(rows), nil
}