package containerd

import (
	"sync"
	"time"

	"github.com/oakestra/oakestra/go_node_engine/model"
)

// Container represents a container managed by the containerd runtime.
type Container struct {
	mu         sync.Mutex
	ID         string
	Image      string
	Status     model.ContainerStatus
	CreateTime time.Time
	Runtime    *Runtime
}

// GetID returns the container ID.
func (c *Container) GetID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ID
}

// GetStatus returns the container status.
func (c *Container) GetStatus() model.ContainerStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Status
}

// GetImage returns the container image.
func (c *Container) GetImage() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Image
}

// GetCreateTime returns the container creation time.
func (c *Container) GetCreateTime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.CreateTime
}

// UpdateStatus updates the container status.
func (c *Container) UpdateStatus(status model.ContainerStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Status = status
}
