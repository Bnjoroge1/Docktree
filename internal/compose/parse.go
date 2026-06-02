package compose


func PortRequests(project *ComposeProject, defaultHostIP string) []PortMapping {
	var requests []PortMapping
	for _, svc := range project.Services {
		for _, port := range svc.Ports {
			if port.Published == 0 {
				continue
			}
			if port.HostIP == "" {
				port.HostIP = defaultHostIP
			}
			requests = append(requests, port)
		}
	}
	return requests
}

func normalizeProtocol(value string) string {
	if value == "" {
		return "tcp"
	}
	return value
}
