package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bnjoroge/docktree/internal/docker"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
)

type simpleHelpOptions struct {
	help bool
}

func parseNoArgHelpOptions(command string, args []string) (simpleHelpOptions, error) {
	var options simpleHelpOptions
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			options.help = true
			return options, nil
		default:
			return simpleHelpOptions{}, fmt.Errorf("unknown %s flag %q", command, arg)
		}
	}
	return options, nil
}

type portsOptions struct {
	all  bool
	help bool
}

func parsePortsOptions(args []string) (portsOptions, error) {
	var options portsOptions
	for _, arg := range args {
		switch arg {
		case "-a", "--all", "-all":
			options.all = true
		case "-h", "--help":
			options.help = true
		default:
			return portsOptions{}, fmt.Errorf("unknown ports flag %q", arg)
		}
	}
	return options, nil
}

type volumesOptions struct {
	all  bool
	help bool
}

func parseVolumesOptions(args []string) (volumesOptions, error) {
	var options volumesOptions
	for _, arg := range args {
		switch arg {
		case "-a", "--all", "-all":
			options.all = true
		case "-h", "--help":
			options.help = true
		default:
			return volumesOptions{}, fmt.Errorf("unknown volumes flag %q", arg)
		}
	}
	return options, nil
}

type cleanOptions struct {
	help    bool
	dryRun  bool
	yes     bool
	volumes bool
}

type cleanCandidate struct {
	Name       string
	Reason     string
	Ports      int
	Resources  docker.ProjectResources
	Instance   *state.Instance
	StateFound bool
}

type downOptions struct {
	help     bool
	dryRun   bool
	volumes  bool
	all      bool
	services []string
}

func parseDownOptions(args []string) (downOptions, error) {
	var options downOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			options.help = true
			return options, nil
		case arg == "--dry-run":
			options.dryRun = true
		case arg == "-v" || arg == "--volumes":
			options.volumes = true
		case arg == "-a" || arg == "--all":
			options.all = true
		default:
			if strings.HasPrefix(arg, "-") {
				return downOptions{}, fmt.Errorf("unknown down flag %q", arg)
			}
			options.services = append(options.services, arg)
		}
	}
	return options, nil
}

type stopOptions struct {
	help     bool
	dryRun   bool
	services []string
}

func parseStopOptions(args []string) (stopOptions, error) {
	var options stopOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			options.help = true
			return options, nil
		case arg == "--dry-run":
			options.dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return stopOptions{}, fmt.Errorf("unknown stop flag %q", arg)
			}
			options.services = append(options.services, arg)
		}
	}
	return options, nil
}

type upOptions struct {
	help     bool
	file     string
	create   string
	sync     bool
	validate bool
	dryRun   bool
	build    bool
}

type createOptions struct {
	help   bool
	branch string
}

func parseUpOptions(args []string) (upOptions, error) {
	var options upOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			options.help = true
			return options, nil
		case arg == "-f" || arg == "--file":
			if i+1 >= len(args) {
				return upOptions{}, fmt.Errorf("%s requires a value", arg)
			}
			options.file = args[i+1]
			i++
		case strings.HasPrefix(arg, "--file="):
			options.file = strings.TrimPrefix(arg, "--file=")
		case arg == "--create":
			if i+1 >= len(args) {
				return upOptions{}, fmt.Errorf("%s requires a branch name", arg)
			}
			options.create = args[i+1]
			i++
		case strings.HasPrefix(arg, "--create="):
			options.create = strings.TrimPrefix(arg, "--create=")
		case arg == "--sync":
			options.sync = true
		case arg == "--validate":
			options.validate = true
		case arg == "--dry-run":
			options.dryRun = true
		case arg == "--build":
			options.build = true
		default:
			return upOptions{}, fmt.Errorf("unknown up flag %q", arg)
		}
	}
	return options, nil
}

func parseCreateOptions(args []string) (createOptions, error) {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return createOptions{help: true}, nil
	}
	if len(args) != 1 {
		return createOptions{}, fmt.Errorf("usage: docktree create <branch>")
	}
	if strings.HasPrefix(args[0], "-") {
		return createOptions{}, fmt.Errorf("usage: docktree create <branch>")
	}
	return createOptions{branch: args[0]}, nil
}

func parseCleanOptions(args []string) (cleanOptions, error) {
	var options cleanOptions
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			options.help = true
			return options, nil
		case "--dry-run":
			options.dryRun = true
		case "--yes":
			options.yes = true
		case "--volumes":
			options.volumes = true
		default:
			return cleanOptions{}, fmt.Errorf("unknown clean flag %q", arg)
		}
	}
	return options, nil
}

func parseGlobalFlags(args []string) (bool, []string) {
	jsonMode := false
	var rest []string
	for _, arg := range args {
		if arg == "--json" {
			jsonMode = true
			continue
		}
		rest = append(rest, arg)
	}
	return jsonMode, rest
}

func sortedKeys(m map[string][]ports.Assignment) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
