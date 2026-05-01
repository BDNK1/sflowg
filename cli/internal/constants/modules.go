package constants

// Module paths for SFlowG components
const (
	// BaseModulePath is the root path for all SFlowG modules
	BaseModulePath = "github.com/BDNK1/sflowg"

	// RuntimeModulePath is the full path to the core runtime module
	RuntimeModulePath = BaseModulePath + "/core"

	// PluginsBasePath is the base path for core plugins
	PluginsBasePath = BaseModulePath + "/plugins"

	// HTTPTransportModulePath is the external HTTP transport module.
	HTTPTransportModulePath = BaseModulePath + "/transports/http"

	// KafkaTransportModulePath is the external Kafka transport module.
	KafkaTransportModulePath = BaseModulePath + "/transports/kafka"

	// CronTransportModulePath is the external Cron transport module.
	CronTransportModulePath = BaseModulePath + "/transports/cron"
)

// Application runtime defaults
const (
	// DefaultPort is the default HTTP server port
	DefaultPort = "8080"
)
