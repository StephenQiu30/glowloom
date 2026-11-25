package domain

// ContainerConfig distinguishes durable group and stack presentation over the parent tree.
type ContainerConfig struct {
	Version    int    `json:"version"`
	Kind       string `json:"kind" enums:"group,stack"`
	Background string `json:"background" enums:"none,cyan,blue,purple,orange,yellow"`
}

func validContainer(c *ContainerConfig) bool {
	if c == nil {
		return true
	}
	if c.Version != 1 || (c.Kind != "group" && c.Kind != "stack") {
		return false
	}
	switch c.Background {
	case "none", "cyan", "blue", "purple", "orange", "yellow":
		return true
	default:
		return false
	}
}
