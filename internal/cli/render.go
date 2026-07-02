package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/tui"
)

// termWidth returns the terminal width for the given writer, or 0 if unknown.
func termWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		return tui.GetTerminalWidthFrom(f)
	}
	return 0
}

func humanRenderer() func(io.Writer, any) {
	return func(w io.Writer, data any) {
		tw := termWidth(w)
		switch v := data.(type) {
		case UpResult:
			projectName := v.Instance.ProjectName
			if v.AlreadyRunning {
				if v.Synced {
					fmt.Fprintf(w, "%s %s %s\n",
						tui.OKS("✓"), tui.MutedS("Synced"), tui.AccentS(projectName))
				} else {
					fmt.Fprintf(w, "%s %s is already running.\n",
						tui.BrandS("Docktree"), tui.AccentS(projectName))
				}
				if len(v.Ports) > 0 {
					fmt.Fprintln(w)
					renderPortList(w, v.Ports)
				}
				return
			}

			fmt.Fprintf(w, "%s Started %s", tui.OKS("✓"), tui.AccentS(projectName))
			if v.Synced {
				fmt.Fprintf(w, " %s", tui.Badge("synced", "SYNCED"))
			}
			fmt.Fprintln(w)

			if len(v.Ports) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintln(w, tui.DimS("Allocated ports"))
				renderPortList(w, v.Ports)
			}
			if len(v.SharedServices) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s  %s\n",
					tui.DimS("Platform services:"),
					tui.InfoS(strings.Join(v.SharedServices, ", ")))
			}

			if v.CreatedWorktree != "" || len(v.ComposeFiles) > 0 || v.OverrideFile != "" {
				fmt.Fprintln(w)
			}
			if v.CreatedWorktree != "" {
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Created worktree"), v.CreatedWorktree)
			}
			if len(v.ComposeFiles) > 0 {
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Compose files:"), strings.Join(v.ComposeFiles, ", "))
			}
			if v.OverrideFile != "" {
				fmt.Fprintf(w, "%s       %s\n", tui.DimS("Override:"), v.OverrideFile)
			}

			if v.Scaffolded {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s Scaffolded %s\n",
					tui.OKS("✓"), tui.AccentS("docktree.yml"))
			}
			for _, warning := range v.EnvWarnings {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s %s\n",
					tui.WarningS("⚠ Warning:"), tui.DimS(warning.Message))
			}
			if len(v.StaleCopies) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s %s\n",
					tui.WarningS("⚠ Stale copies:"), strings.Join(v.StaleCopies, ", "))
				fmt.Fprintf(w, "  %s\n", tui.DimS("Run `docktree sync` to update"))
			}
			if len(v.IsolatedVolumes) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s %s\n",
					tui.DimS("Isolated volumes:"), strings.Join(v.IsolatedVolumes, ", "))
			}
			if v.Hint != "" {
				fmt.Fprintln(w)
				fmt.Fprintln(w, tui.DimS(v.Hint))
			}
		case DownResult:
			if len(v.DroppedTenants) > 0 {
				for _, db := range v.DroppedTenants {
					fmt.Fprintf(w, "%s Dropped tenant database: %s\n",
						tui.WarningS("!"), tui.MutedS(db))
				}
			}
			if v.AlreadyStopped {
				if v.Instance != nil {
					fmt.Fprintf(w, "%s %s is already stopped.\n",
						tui.BrandS("Docktree"), tui.AccentS(v.Instance.ProjectName))
				} else {
					fmt.Fprintf(w, "%s is already stopped.\n", tui.BrandS("Docktree"))
				}
				return
			}
			if v.DryRun {
				if v.Instance == nil {
					fmt.Fprintln(w, "Docktree dry run - would stop all matching instances")
					if len(v.Services) > 0 {
						fmt.Fprintf(w, "  Instances: %s\n", strings.Join(v.Services, ", "))
					}
					return
				}
				fmt.Fprintf(w, "Docktree dry run - would stop %s\n", v.Instance.ProjectName)
				fmt.Fprintf(w, "  Services: %s\n", strings.Join(v.Services, ", "))
				fmt.Fprintf(w, "  Compose files:\n")
				for _, f := range v.ComposeFiles {
					fmt.Fprintf(w, "    %s\n", f)
				}
				return
			}
			if v.Instance == nil {
				fmt.Fprintln(w, "Docktree stopped all matching instances")
				if len(v.Services) > 0 {
					fmt.Fprintf(w, "  Instances: %s\n", strings.Join(v.Services, ", "))
				}
				return
			}
			fmt.Fprintf(w, "Docktree stopped %s\n", v.Instance.ProjectName)
			if len(v.Services) > 0 {
				fmt.Fprintf(w, "  Services: %s\n", strings.Join(v.Services, ", "))
			}
		case StopResult:
			if v.AlreadyStopped {
				fmt.Fprintln(w, "Docktree is already stopped.")
				return
			}
			if v.DryRun {
				fmt.Fprintf(w, "Docktree dry run - would stop %s (containers only, not removed)\n", v.Instance.ProjectName)
				fmt.Fprintf(w, "  Services: %s\n", strings.Join(v.Services, ", "))
				fmt.Fprintf(w, "  Compose files:\n")
				for _, f := range v.ComposeFiles {
					fmt.Fprintf(w, "    %s\n", f)
				}
				return
			}
			fmt.Fprintf(w, "Docktree stopped %s (containers only, not removed)\n", v.Instance.ProjectName)
			if len(v.Services) > 0 {
				fmt.Fprintf(w, "  Services: %s\n", strings.Join(v.Services, ", "))
			}
		case LsResult:
			if len(v.Entries) == 0 {
				fmt.Fprintf(w, "%s No Compose projects found.\n", tui.BrandS("Docktree"))
				return
			}
			var tbl tui.Table
			tbl.TermWidth = tw
			tbl.Headers = []string{"NAME", "STATUS", "CONFIG FILES"}
			for _, e := range v.Entries {
				tbl.Rows = append(tbl.Rows, []string{e.Name, e.Status, shortenConfigFiles(e.ConfigFiles)})
			}
			fmt.Fprint(w, tbl.RenderBordered())
		case ImagesResult:
			if len(v.Entries) == 0 {
				fmt.Fprintf(w, "%s No images found for this project.\n", tui.BrandS("Docktree"))
				return
			}
			var tbl tui.Table
			tbl.TermWidth = tw
			tbl.Headers = []string{"CONTAINER", "REPOSITORY", "TAG", "PLATFORM", "IMAGE ID", "SIZE", "CREATED"}
			for _, e := range v.Entries {
				platform := e.Platform
				if platform == "" {
					platform = "—"
				}
				container := e.ContainerName
				if v.ProjectName != "" && strings.HasPrefix(container, v.ProjectName+"-") {
					container = container[len(v.ProjectName)+1:]
				}
				tbl.Rows = append(tbl.Rows, []string{
					container,
					shortenRepository(e.Repository),
					e.Tag,
					platform,
					shortImageID(e.ID),
					formatBytes(e.Size),
					relativeTime(e.Created),
				})
			}
			fmt.Fprint(w, tbl.RenderBordered())
		case TopResult:
			if len(v.Rows) == 0 {
				fmt.Fprintf(w, "%s No running containers.\n", tui.BrandS("Docktree"))
				return
			}
			var tbl tui.Table
			tbl.TermWidth = tw
			tbl.Headers = []string{"SERVICE", "#", "UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"}
			for _, r := range v.Rows {
				tbl.Rows = append(tbl.Rows, []string{
					r.Service, r.Num, r.UID, r.PID, r.PPID,
					r.CPU, r.STime, r.TTY, r.Time, r.Cmd,
				})
			}
			fmt.Fprint(w, tbl.RenderBordered())
		case ComposePassthroughResult:
			// Output already streamed by docker compose; nothing to render in human mode.
		case ValidateResult:
			if v.Valid {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.OKS("config is valid"))
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Services:"), strings.Join(v.Services, ", "))
				if len(v.Profiles) > 0 {
					fmt.Fprintf(w, "%s  %s\n", tui.DimS("Profiles:"), strings.Join(v.Profiles, ", "))
				}
				if len(v.SkippedServices) > 0 {
					fmt.Fprintf(w, "%s  %s\n", tui.DimS("Skipped:"), strings.Join(v.SkippedServices, ", "))
				}
				if len(v.DroppedDependencies) > 0 {
					fmt.Fprintf(w, "%s  %s\n", tui.DimS("Dropped deps:"), strings.Join(v.DroppedDependencies, ", "))
				}
				if len(v.Ports) > 0 {
					fmt.Fprintln(w)
					fmt.Fprintf(w, "%s\n", tui.DimS("Ports:"))
					for _, a := range v.Ports {
						fmt.Fprintf(w, "  %-14s%s %s %s\n",
							tui.TextS(a.Service),
							tui.MutedS(fmt.Sprintf("%d", a.ContainerPort)),
							tui.DimS("→"),
							tui.AccentS(fmt.Sprintf("%d", a.HostPort)))
					}
				}
				if len(v.IsolatedVolumes) > 0 {
					fmt.Fprintln(w)
					fmt.Fprintf(w, "%s  %s\n",
						tui.DimS("Isolated volumes:"), strings.Join(v.IsolatedVolumes, ", "))
				}
			} else {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.ErrorS("config has errors:"))
				for _, e := range v.Errors {
					fmt.Fprintf(w, "  %s %s\n", tui.ErrorS("✗"), e)
				}
			}
			for _, warning := range v.EnvWarnings {
				fmt.Fprintf(w, "  %s %s: %s\n",
					tui.WarningS("⚠"), tui.DimS(warning.Key), warning.Message)
			}
		case DryRunResult:
			fmt.Fprintf(w, "%s %s %s\n",
				tui.BrandS("Docktree"), tui.MutedS("dry run for"), tui.AccentS(v.InstanceName))
			fmt.Fprintln(w)
			fmt.Fprintf(w, "%s  %s\n", tui.DimS("Services:"), strings.Join(v.Services, ", "))
			if len(v.Profiles) > 0 {
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Profiles:"), strings.Join(v.Profiles, ", "))
			}
			if len(v.SkippedServices) > 0 {
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Skipped:"), strings.Join(v.SkippedServices, ", "))
			}
			if len(v.DroppedDependencies) > 0 {
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Dropped deps:"), strings.Join(v.DroppedDependencies, ", "))
			}
			fmt.Fprintf(w, "%s\n", tui.DimS("Compose files:"))
			for _, f := range v.ComposeFiles {
				fmt.Fprintf(w, "  %s\n", tui.AccentS(f))
			}
			if len(v.Ports) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.DimS("Port assignments:"))
				for _, a := range v.Ports {
					fmt.Fprintf(w, "  %-14s%s %s %s\n",
						tui.TextS(a.Service),
						tui.MutedS(fmt.Sprintf("%d", a.ContainerPort)),
						tui.DimS("→"),
						tui.AccentS(fmt.Sprintf("%d", a.HostPort)))
				}
			}
			if len(v.IsolatedVolumes) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s  %s\n",
					tui.DimS("Isolated volumes:"), strings.Join(v.IsolatedVolumes, ", "))
			}
			if v.OverridePreview != "" {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.WarningS("Override preview:"))
				fmt.Fprintln(w, v.OverridePreview)
			}
			if v.ClearPreview != "" {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.DimS("Port clear preview:"))
				fmt.Fprintln(w, v.ClearPreview)
			}
			for _, warning := range v.EnvWarnings {
				fmt.Fprintf(w, "  %s %s: %s\n",
					tui.WarningS("⚠"), tui.DimS(warning.Key), warning.Message)
			}
		case StatusResult:
			if v.Stopped {
				fmt.Fprintf(w, "%s is stopped.\n", tui.BrandS("Docktree"))
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.MutedS("Run `docktree up` to start this worktree."))
				return
			}
			if v.Raw != nil {
				var services []composePsEntry
				_ = json.Unmarshal(v.Raw, &services)
				if len(services) == 0 {
					fmt.Fprintf(w, "%s %s  %s\n",
						tui.ErrorS("●"), tui.AccentS(v.Instance.ProjectName), tui.Badge("stopped", "STOPPED"))
					fmt.Fprintf(w, "%s  %s\n",
						tui.DimS("Branch:"), tui.MutedS(v.Instance.Branch))
					fmt.Fprintln(w)
					fmt.Fprintf(w, "%s\n", tui.MutedS("Run `docktree up` to start services."))
					return
				}
				if true {
					running := 0
					paused := 0
					for _, s := range services {
						switch {
						case strings.EqualFold(s.State, "running"):
							running++
						case strings.EqualFold(s.State, "paused"):
							paused++
						}
					}
					statusLabel := tui.OKS("running")
					statusBadge := tui.Badge("ok", "RUNNING")
					if running == 0 && paused == len(services) && len(services) > 0 {
						statusLabel = tui.WarningS("paused")
						statusBadge = tui.Badge("warning", "PAUSED")
					} else if running < len(services) && running > 0 {
						statusLabel = tui.WarningS("partial")
						statusBadge = tui.Badge("warning", "PARTIAL")
					} else if running == 0 {
						statusLabel = tui.ErrorS("stopped")
						statusBadge = tui.Badge("error", "STOPPED")
					}
					_ = statusLabel

					if v.Instance != nil {
						fmt.Fprintf(w, "%s %s  %s\n",
							tui.OKS("●"), tui.AccentS(v.Instance.ProjectName), statusBadge)
						fmt.Fprintf(w, "%s  %s    %s\n",
							tui.DimS("Branch:"), tui.MutedS(v.Instance.Branch),
							tui.MutedS(fmt.Sprintf("%d/%d services", running, len(services))))
					}
					fmt.Fprintln(w)

					var svcTbl tui.Table
					svcTbl.Headers = []string{"SERVICE", "IMAGE", "STATE", "STATUS"}
					for _, s := range services {
						img := shortenImage(s.Image)
						status := s.Status
						if status == "" {
							status = "—"
						}
						svcTbl.Rows = append(svcTbl.Rows, []string{s.Service, img, s.State, status})
					}
					fmt.Fprintln(w, svcTbl.RenderBorderedStyled(func(row, col int, val string) string {
						if row == -1 {
							return tui.DimS(val)
						}
						switch col {
						case 0:
							return tui.TextS(val)
						case 1:
							return tui.MutedS(val)
						case 2:
							switch {
							case strings.EqualFold(val, "running"):
								return tui.OKS(val)
							case strings.EqualFold(val, "paused"):
								return tui.WarningS(val)
							case strings.EqualFold(val, "exited"), strings.EqualFold(val, "restarting"):
								return tui.ErrorS(val)
							default:
								return tui.WarningS(val)
							}
						case 3:
							return tui.DimS(val)
						}
						return val
					}))

					var hasPublishers bool
					for _, s := range services {
						if len(s.Publishers) > 0 {
							for _, p := range s.Publishers {
								if p.PublishedPort > 0 {
									hasPublishers = true
									break
								}
							}
						}
						if hasPublishers {
							break
						}
					}
					if hasPublishers {
						fmt.Fprintln(w)
						var portTbl tui.Table
						portTbl.Headers = []string{"SERVICE", "PORT", "URL"}
						for _, s := range services {
							for _, p := range s.Publishers {
								if p.PublishedPort > 0 {
									url := fmt.Sprintf("http://%s:%d", p.URL, p.PublishedPort)
									portTbl.Rows = append(portTbl.Rows, []string{
										s.Service,
										fmt.Sprintf("%d", p.PublishedPort),
										url,
									})
								}
							}
						}
						fmt.Fprintln(w, portTbl.RenderBorderedStyled(func(row, col int, val string) string {
							if row == -1 {
								return tui.DimS(val)
							}
							switch col {
							case 0:
								return tui.OKS(val)
							case 1:
								return tui.AccentS(val)
							case 2:
								return tui.URLS(val)
							}
							return val
						}))
					}
					return
				}
			}
			if v.Instance != nil {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.AccentS(v.Instance.ProjectName))
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Branch:"), v.Instance.Branch)
			}
			if v.Text != "" {
				fmt.Fprintln(w, v.Text)
			}
		case StatusAllResult:
			if len(v.Entries) == 0 {
				fmt.Fprintf(w, "%s No worktree instances found.\n", tui.BrandS("Docktree"))
				return
			}
			fmt.Fprintf(w, "%s\n", tui.BrandS("Docktree"))
			fmt.Fprintln(w)
			var tbl tui.Table
			tbl.TermWidth = tw
			tbl.Headers = []string{"", "INSTANCE", "BRANCH", "SERVICES", "PROXY URL"}
			for _, e := range v.Entries {
				statusIcon := tui.ErrorS("○")
				if e.Running {
					statusIcon = tui.OKS("●")
				} else if e.Paused {
					statusIcon = tui.WarningS("◌")
				}
				services := fmt.Sprintf("%d/%d", e.RunningCount, e.TotalServices)
				if e.TotalServices == 0 {
					services = "—"
				}
				proxyURL := e.ProxyURL
				if proxyURL == "" {
					proxyURL = "—"
				}
				tbl.Rows = append(tbl.Rows, []string{
					statusIcon,
					e.Instance,
					e.Branch,
					services,
					proxyURL,
				})
			}
			fmt.Fprintln(w, tbl.RenderBorderedStyled(func(row, col int, val string) string {
				if row == -1 {
					return tui.DimS(val)
				}
				switch col {
				case 0:
					return val // already styled as icon
				case 1:
					return tui.MutedS(val)
				case 2:
					return tui.AccentS(val)
				case 3:
					return tui.TextS(val)
				case 4:
					if val == "—" {
						return tui.DimS(val)
					}
					return tui.URLS(val)
				}
				return val
			}))

		case PortsResult:
			if v.All {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("ports (all instances)"))
			} else {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.AccentS(v.Instance))
			}
			if len(v.Entries) == 0 {
				break
			}
			fmt.Fprintln(w)
			var tbl tui.Table
			tbl.TermWidth = tw
			if v.All && len(v.Entries) > 1 {
				tbl.Headers = []string{"INSTANCE", "SERVICE", "CONTAINER", "HOST", "BIND", "URL"}
				for _, entry := range v.Entries {
					for _, a := range entry.Ports {
						url := fmt.Sprintf("http://%s:%d", a.HostIP, a.HostPort)
						tbl.Rows = append(tbl.Rows, []string{
							entry.Instance, a.Service,
							fmt.Sprintf("%d", a.ContainerPort), fmt.Sprintf("%d", a.HostPort),
							a.HostIP, url,
						})
					}
				}
			} else {
				tbl.Headers = []string{"SERVICE", "CONTAINER", "HOST", "BIND", "URL"}
				for _, entry := range v.Entries {
					for _, a := range entry.Ports {
						url := fmt.Sprintf("http://%s:%d", a.HostIP, a.HostPort)
						tbl.Rows = append(tbl.Rows, []string{
							a.Service,
							fmt.Sprintf("%d", a.ContainerPort), fmt.Sprintf("%d", a.HostPort),
							a.HostIP, url,
						})
					}
				}
			}
			fmt.Fprintln(w, tbl.RenderBorderedStyled(func(row, col int, val string) string {
				if row == -1 {
					return tui.DimS(val)
				}
				if len(tbl.Headers) == 6 {
					switch col {
					case 0:
						return tui.MutedS(val)
					case 1:
						return tui.OKS(val)
					case 2:
						return tui.DimS(val)
					case 3:
						return tui.AccentS(val)
					case 5:
						return tui.URLS(val)
					}
				} else {
					switch col {
					case 0:
						return tui.OKS(val)
					case 1:
						return tui.DimS(val)
					case 2:
						return tui.AccentS(val)
					case 4:
						return tui.URLS(val)
					}
				}
				return val
			}))
		case VolumesResult:
			if v.All {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("volumes (all instances)"))
			} else {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.AccentS(v.Instance))
			}
			if len(v.Entries) == 0 {
				break
			}
			fmt.Fprintln(w)
			var tbl tui.Table
			tbl.TermWidth = tw
			if v.All {
				tbl.Headers = []string{"INSTANCE", "VOLUME", "DRIVER", "DOCKER NAME"}
				for _, entry := range v.Entries {
					tbl.Rows = append(tbl.Rows, []string{
						entry.Instance, entry.Volume, entry.Driver, entry.Name,
					})
				}
			} else {
				tbl.Headers = []string{"VOLUME", "DRIVER", "DOCKER NAME"}
				for _, entry := range v.Entries {
					tbl.Rows = append(tbl.Rows, []string{
						entry.Volume, entry.Driver, entry.Name,
					})
				}
			}
			fmt.Fprintln(w, tbl.RenderBorderedStyled(func(row, col int, val string) string {
				if row == -1 {
					return tui.DimS(val)
				}
				if len(tbl.Headers) == 4 {
					switch col {
					case 0:
						return tui.MutedS(val)
					case 1:
						return tui.OKS(val)
					case 2:
						return tui.AccentS(val)
					case 3:
						return tui.DimS(val)
					}
				} else {
					switch col {
					case 0:
						return tui.OKS(val)
					case 1:
						return tui.AccentS(val)
					case 2:
						return tui.DimS(val)
					}
				}
				return val
			}))
		case PrepareResult:
			fmt.Fprintf(w, "%s %s %s\n",
				tui.BrandS("Docktree"), tui.MutedS("preparing"), tui.AccentS(v.WorktreeRoot))
			fmt.Fprintln(w)
			fmt.Fprintf(w, "%s    %s\n", tui.DimS("Git repo:"), v.RepoRoot)
			fmt.Fprintf(w, "%s   %s\n", tui.DimS("Worktree:"), v.WorktreeRoot)
			if len(v.Ran) > 0 {
				fmt.Fprintln(w)
				for _, step := range v.Ran {
					fmt.Fprintf(w, "  %s %s\n", tui.OKS("✓"), tui.MutedS(step))
				}
			}
		case CreateResult:
			fmt.Fprintf(w, "%s created worktree %s for %s\n",
				tui.BrandS("Docktree"), tui.AccentS(v.WorktreeRoot), tui.AccentS(v.Branch))
			fmt.Fprintln(w)
			fmt.Fprintf(w, "  %s    %s\n", tui.DimS("Git worktree"), tui.MutedS(v.Branch))
			fmt.Fprintf(w, "  %s            %s\n", tui.DimS("Path"), tui.MutedS(v.WorktreeRoot))
			if len(v.Ran) > 0 {
				fmt.Fprintln(w)
				for _, step := range v.Ran {
					fmt.Fprintf(w, "  %s %s\n", tui.OKS("✓"), tui.MutedS(step))
				}
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "%s %s %s\n",
				tui.MutedS("Run"), tui.AccentS("docktree up"), tui.MutedS("in the new worktree to start services."))
		case CleanResult:
			if len(v.Instances) == 0 {
				fmt.Fprintf(w, "%s found no stale resources.\n", tui.BrandS("Docktree"))
				return
			}
			var tbl tui.Table
			tbl.TermWidth = tw
			tbl.Headers = []string{"INSTANCE", "REASON", "RESOURCES"}
			for _, item := range v.Instances {
				resources := fmt.Sprintf("%d ports, %d containers, %d networks", item.Ports, item.Containers, item.Networks)
				if v.Volumes {
					resources = fmt.Sprintf("%s, %d volumes", resources, item.Volumes)
				}
				tbl.Rows = append(tbl.Rows, []string{
					item.Instance,
					item.Reason,
					resources,
				})
			}
			renderedTable := tbl.RenderBorderedStyled(func(row, col int, val string) string {
				if row == -1 {
					return tui.DimS(val)
				}
				switch col {
				case 0:
					return tui.MutedS(val)
				case 1:
					return tui.DimS(val)
				case 2:
					return tui.MutedS(val)
				}
				return val
			})

			if v.DryRun {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("dry run — nothing will be removed"))
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.MutedS("Would remove:"))
				fmt.Fprintln(w, renderedTable)
				fmt.Fprintln(w)
				totalsStr := fmt.Sprintf("%d ports, %d containers, %d networks", v.Totals.Ports, v.Totals.Containers, v.Totals.Networks)
				if v.Volumes {
					totalsStr = fmt.Sprintf("%s, %d volumes", totalsStr, v.Totals.Volumes)
				}
				fmt.Fprintf(w, "%s  %s\n", tui.MutedS("Total:"), tui.MutedS(totalsStr))
				return
			}
			if v.Removed {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("removed stale resources"))
			} else {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("scanning for stale resources..."))
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w, renderedTable)
			if v.Removed {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "  %s %s\n", tui.OKS("✓"), tui.MutedS("Removed stale resources"))
				fmt.Fprintf(w, "%s\n",
					tui.MutedS(fmt.Sprintf("%d ports freed. %d instances removed.",
						v.Totals.Ports, v.Totals.Instances)))
			}
		case PlatformResult:
			if v.Skipped {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS(v.Reason))
				return
			}
			switch v.Action {
			case "up":
				if v.Running {
					fmt.Fprintf(w, "%s Platform %s\n", tui.OKS("✓"), tui.AccentS(v.Project))
				}
			case "down":
				fmt.Fprintf(w, "%s Stopped platform %s\n", tui.OKS("✓"), tui.AccentS(v.Project))
			case "status":
				state := "stopped"
				if v.Running {
					state = "running"
				}
				fmt.Fprintf(w, "%s Platform %s  %s\n",
					tui.BrandS("Docktree"), tui.AccentS(v.Project), tui.Badge(state, strings.ToUpper(state)))
				fmt.Fprintf(w, "  %s  %s\n", tui.DimS("network:"), tui.MutedS(v.Network))
				for _, svc := range v.Services {
					fmt.Fprintf(w, "  %s  %s\n", tui.DimS("service:"), tui.OKS(svc))
				}
			case "clean":
				if v.DryRun {
					fmt.Fprintf(w, "%s Dry run — platform %s\n", tui.BrandS("Docktree"), tui.AccentS(v.Project))
					fmt.Fprintf(w, "  %s  %s\n", tui.DimS("would stop:"), v.Project)
					fmt.Fprintf(w, "  %s  %s\n", tui.DimS("would remove:"), v.Network)
					if len(v.WouldDrop) > 0 {
						fmt.Fprintf(w, "  %s\n", tui.DimS("would drop databases:"))
						for _, db := range v.WouldDrop {
							fmt.Fprintf(w, "    %s\n", tui.MutedS(db))
						}
					}
					return
				}
				fmt.Fprintf(w, "%s Platform %s cleaned\n", tui.OKS("✓"), tui.AccentS(v.Project))
				if len(v.DroppedDatabases) > 0 {
					fmt.Fprintf(w, "  %s\n", tui.DimS("dropped databases:"))
					for _, db := range v.DroppedDatabases {
						fmt.Fprintf(w, "    %s %s\n", tui.OKS("✓"), tui.MutedS(db))
					}
				}
			}
		case PlatformTenantsResult:
			if len(v.Tenants) == 0 {
				fmt.Fprintf(w, "%s No tenant databases found.\n", tui.BrandS("Docktree"))
				return
			}
			var tbl tui.Table
			tbl.TermWidth = tw
			tbl.Headers = []string{"INSTANCE", "SERVICE", "LOGICAL DB", "TENANT DB", "EXISTS"}
			for _, e := range v.Tenants {
				existsStr := tui.OKS("yes")
				if !e.Exists {
					existsStr = tui.WarningS("no")
				}
				logical := e.LogicalDB
				if logical == "" {
					logical = "default"
				}
				tbl.Rows = append(tbl.Rows, []string{
					truncate(e.Instance, 35),
					e.Service,
					truncate(logical, 18),
					truncate(e.TenantDB, 40),
					existsStr,
				})
			}
			fmt.Fprintln(w, tbl.RenderBorderedStyled(func(row, col int, val string) string {
				if row == -1 {
					return tui.DimS(val)
				}
				switch col {
				case 0:
					return tui.MutedS(val)
				case 1:
					return tui.AccentS(val)
				case 2, 3:
					return tui.TextS(val)
				}
				return val
			}))
		case SyncResult:
			if len(v.Items) == 0 {
				fmt.Fprintf(w, "%s Everything is in sync.\n", tui.BrandS("Docktree"))
				return
			}
			syncedCount := 0
			skippedCount := 0
			for _, item := range v.Items {
				if item.Skipped != "" {
					skippedCount++
				} else {
					syncedCount++
				}
			}
			if v.Synced && skippedCount > 0 {
				fmt.Fprintf(w, "%s Synced %d file(s), skipped %d worktree(s)\n",
					tui.OKS("✓"), countFilesFromItems(v.Items), skippedCount)
			} else if v.Synced {
				fmt.Fprintf(w, "%s Synced %d file(s) across %d worktree(s)\n",
					tui.OKS("✓"), countFilesFromItems(v.Items), len(v.Items))
			} else if skippedCount > 0 {
				fmt.Fprintf(w, "%s Skipped %d worktree(s) (running)\n",
					tui.WarningS("⚠"), skippedCount)
			} else {
				fmt.Fprintf(w, "%s Would sync %d file(s) across %d worktree(s)\n",
					tui.MutedS("dry run"), countFilesFromItems(v.Items), len(v.Items))
			}
			fmt.Fprintln(w)
			var tbl tui.Table
			tbl.TermWidth = tw
			tbl.Headers = []string{"INSTANCE", "BRANCH", "FILES"}
			for _, item := range v.Items {
				files := strings.Join(item.Files, ", ")
				if item.Skipped != "" {
					files = tui.WarningS(item.Skipped)
				} else {
					files = tui.OKS(files)
				}
				tbl.Rows = append(tbl.Rows, []string{
					truncate(item.Instance, 35),
					item.Branch,
					files,
				})
			}
			fmt.Fprintln(w, tbl.RenderBorderedStyled(func(row, col int, val string) string {
				if row == -1 {
					return tui.DimS(val)
				}
				switch col {
				case 0:
					return tui.MutedS(val)
				case 1:
					return tui.AccentS(val)
				case 2:
					return val // already styled
				}
				return val
			}))

		case HelpDoc:
			renderHelpText(w, v)

		case VersionInfo:
			renderVersionText(w, v)
		case TunnelStartResult:
			// Already printed by the command itself via Steps; nothing to add here.

		case TunnelListResult:
			if len(v.Entries) == 0 {
				fmt.Fprintf(w, "%s No tunnels running.\n", tui.BrandS("Docktree"))
				return
			}
			fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("tunnels"))
			fmt.Fprintln(w)
			var tbl tui.Table
			tbl.TermWidth = tw
			tbl.Headers = []string{"", "INSTANCE", "BRANCH", "PROVIDER", "URL", "PORT"}
			for _, e := range v.Entries {
				statusIcon := tui.ErrorS("○")
				if e.Alive {
					statusIcon = tui.OKS("●")
				}
				url := e.URL
				if url == "" {
					url = "—"
				}
				tbl.Rows = append(tbl.Rows, []string{
					statusIcon, e.Instance, e.Branch, e.Provider, url, fmt.Sprintf("%d", e.Port),
				})
			}
			fmt.Fprintln(w, tbl.RenderBorderedStyled(func(row, col int, val string) string {
				if row == -1 {
					return tui.DimS(val)
				}
				switch col {
				case 0:
					return val
				case 1:
					return tui.MutedS(val)
				case 2:
					return tui.AccentS(val)
				case 3:
					return tui.TextS(val)
				case 4:
					if val == "—" {
						return tui.DimS(val)
					}
					return tui.URLS(val)
				case 5:
					return tui.AccentS(val)
				}
				return val
			}))

		default:
		}
	}
}

// truncate returns s truncated to max runes, with "…" appended if truncated.
func truncate(s string, max int) string {
	if max <= 3 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
func countFilesFromItems(items []SyncItem) int {
	n := 0
	for _, item := range items {
		n += len(item.Files)
	}
	return n
}

func errorCode(code int) string {
	switch code {
	case output.ExitUsage:
		return "usage"
	case output.ExitConfig:
		return "config"
	case output.ExitDocker:
		return "docker"
	case output.ExitNoop:
		return "noop"
	case output.ExitConflict:
		return "conflict"
	default:
		return "error"
	}
}
func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f kB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// shortImageID strips the "sha256:" prefix and returns the first 12 hex chars.
func shortImageID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		id = id[:12]
	}
	return id
}

// shortenRepository strips the registry/org prefix, keeping just the image name.
// "docktree/docktree-main-4f19b3/api" → "api"
// "postgres" → "postgres"
func shortenRepository(repo string) string {
	if idx := strings.LastIndex(repo, "/"); idx != -1 {
		return repo[idx+1:]
	}
	return repo
}

// relativeTime formats a Docker timestamp as a human-readable relative duration
// (e.g. "4d ago", "2h ago", "just now").
func relativeTime(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		// Fallback: try without nanos, then just return the raw string.
		t, err = time.Parse("2006-01-02T15:04:05Z", ts)
		if err != nil {
			if len(ts) > 10 {
				return ts[:10]
			}
			return ts
		}
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return "—"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

// shortenConfigFiles collapses comma-separated absolute paths into basenames
// for compact display. "a/b/c.yml,d/e/f.yml" → "c.yml, f.yml".
func shortenConfigFiles(cfg string) string {
	if cfg == "" {
		return "—"
	}
	parts := strings.Split(cfg, ",")
	for i, p := range parts {
		parts[i] = filepath.Base(strings.TrimSpace(p))
	}
	return strings.Join(parts, ", ")
}
