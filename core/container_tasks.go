package runtime

func (c *Container) GetTask(name string) Task {
	task, ok := c.tasks[name]
	if !ok {
		return nil
	}
	return task
}

func (c *Container) RangeTasks(fn func(name string, task Task)) {
	for name, task := range c.tasks {
		fn(name, task)
	}
}

func (c *Container) registerTask(name string, task Task) {
	c.tasks[name] = task
}
