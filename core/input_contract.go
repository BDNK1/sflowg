package runtime

import "github.com/BDNK1/sflowg/core/validation/schema"

// InputContract describes the variable namespaces an entrypoint seeds before
// flow steps run. Transports own the meaning of each namespace.
type InputContract struct {
	Namespaces map[string]InputNamespace
}

type InputNamespace struct {
	Root   *schema.Schema
	Fields map[string]*schema.Schema
}

func NewInputContract() *InputContract {
	return &InputContract{Namespaces: make(map[string]InputNamespace)}
}

func (c *InputContract) SetRoot(namespace string, root *schema.Schema) {
	if c.Namespaces == nil {
		c.Namespaces = make(map[string]InputNamespace)
	}
	ns := c.Namespaces[namespace]
	ns.Root = root
	c.Namespaces[namespace] = ns
}

func (c *InputContract) SetFields(namespace string, fields map[string]*schema.Schema) {
	if c.Namespaces == nil {
		c.Namespaces = make(map[string]InputNamespace)
	}
	ns := c.Namespaces[namespace]
	ns.Fields = fields
	c.Namespaces[namespace] = ns
}

func (c *InputContract) RootSchema(namespace string) (*schema.Schema, bool) {
	if c == nil {
		return nil, false
	}
	ns, ok := c.Namespaces[namespace]
	return ns.Root, ok && ns.Root != nil
}

func (c *InputContract) FieldSchemas(namespace string) map[string]*schema.Schema {
	if c == nil {
		return nil
	}
	return c.Namespaces[namespace].Fields
}
