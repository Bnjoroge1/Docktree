package cli

import (
	"slices"
	"strings"

	"github.com/bnjoroge/docktree/internal/docker"
	"github.com/bnjoroge/docktree/internal/output"
)

func runVolumes(ctx *Context) (any, int, error) {
	options, err := parseVolumesOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		printVolumesHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}

	volumesList, err := docker.ListDocktreeVolumes()
	if err != nil {
		return nil, output.ExitDocker, err
	}

	if options.all {
		var entries []VolumesEntry
		for _, v := range volumesList {
			entries = append(entries, VolumesEntry{
				Instance: v.ProjectName,
				Volume:   v.VolumeName,
				Name:     v.Name,
				Driver:   v.Driver,
			})
		}
		// Sort by instance and volume name
		slices.SortFunc(entries, func(a, b VolumesEntry) int {
			if a.Instance != b.Instance {
				return strings.Compare(a.Instance, b.Instance)
			}
			return strings.Compare(a.Volume, b.Volume)
		})
		return VolumesResult{All: true, Entries: entries}, output.ExitOK, nil
	}

	_, _, instanceName, err := commonIdentity()
	if err != nil {
		return nil, output.ExitConfig, err
	}

	var entries []VolumesEntry
	for _, v := range volumesList {
		if v.ProjectName == instanceName {
			entries = append(entries, VolumesEntry{
				Instance: v.ProjectName,
				Volume:   v.VolumeName,
				Name:     v.Name,
				Driver:   v.Driver,
			})
		}
	}
	slices.SortFunc(entries, func(a, b VolumesEntry) int {
		return strings.Compare(a.Volume, b.Volume)
	})
	return VolumesResult{Instance: instanceName, Entries: entries}, output.ExitOK, nil
}
