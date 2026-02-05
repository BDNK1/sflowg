package analyzer

// PluginMetadata contains comprehensive information about a plugin
// extracted via AST analysis
type PluginMetadata struct {
	// Name is the instance name of the plugin (from flow-config.yaml)
	Name string

	// ImportPath is the Go import path (e.g., "github.com/BDNK1/sflowg/plugins/http")
	ImportPath string

	// TypeName is the plugin struct type name (e.g., "HTTPPlugin")
	TypeName string

	// PackageName is the last segment of import path (e.g., "http")
	PackageName string

	// HasConfig indicates if plugin has a Config field
	HasConfig bool

	// ConfigType contains metadata about the plugin's Config struct
	ConfigType *ConfigMetadata

	// Dependencies are other plugins that this plugin depends on
	Dependencies []Dependency

	// Tasks are the task methods discovered on the plugin
	Tasks []TaskMetadata
}

// Dependency represents a plugin dependency detected via struct field
type Dependency struct {
	// FieldName is the struct field name (e.g., "http", "Redis")
	FieldName string

	// PluginType is the full type including pointer (e.g., "*http.HTTPPlugin")
	PluginType string

	// PluginName is the resolved plugin instance name to inject
	// Either derived from field name or from inject tag
	PluginName string

	// InjectTag is the value from inject:"..." tag if present
	InjectTag string

	// IsExported indicates if the field is exported (required for injection)
	IsExported bool
}

// ConfigMetadata contains information about a plugin's Config struct
type ConfigMetadata struct {
	// TypeName is the config struct type name (always "Config")
	TypeName string

	// Fields are the configuration fields with their tags
	Fields []ConfigField
}

// ConfigField represents a single field in a Config struct
type ConfigField struct {
	Name        string
	Type        string
	YAMLTag     string
	DefaultTag  string
	ValidateTag string
}

// TaskMetadata contains information about a plugin task method
type TaskMetadata struct {
	// MethodName is the Go method name (e.g., "Request")
	MethodName string

	// TaskName is the registered task name (e.g., "http.request")
	TaskName string

	// IsExported indicates if method is exported
	IsExported bool

	// HasValidSignature indicates if method matches task signature:
	// func (p *Plugin) Method(exec *plugin.Execution, args map[string]any) (map[string]any, error)
	HasValidSignature bool
}

// AnalysisError represents an error during plugin analysis
type AnalysisError struct {
	PluginName string
	ImportPath string
	Message    string
	Cause      error
}

func (e *AnalysisError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *AnalysisError) Unwrap() error {
	return e.Cause
}
