package ports

// Assignment records the host port selected for a service's container port.
type Assignment struct {
	Service       string `json:"service"`
	ContainerPort int    `json:"container_port"`
	HostIP        string `json:"host_ip"`
	HostPort      int    `json:"host_port"`
}

type PortRequest struct {
	Service       string
	ContainerPort int
	HostIP        string
}

type Range struct {
	Min int
	Max int
}
