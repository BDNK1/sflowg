package core

func (c *Container) GetTask(name string) Task {
	c.mu.RLock()
	defer c.mu.RUnlock()
	task, ok := c.tasks[name]
	if !ok {
		return nil
	}
	return task
}

func (c *Container) RangeTasks(fn func(name string, task Task)) {
	c.mu.RLock()
	snapshot := make(map[string]Task, len(c.tasks))
	for name, task := range c.tasks {
		snapshot[name] = task
	}
	c.mu.RUnlock()
	for name, task := range snapshot {
		fn(name, task)
	}
}

func (c *Container) registerTask(name string, task Task) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tasks[name] = task
}
