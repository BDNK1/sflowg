package plugin

import "github.com/BDNK1/sflowg/runtime"

// Execution is the runtime context passed to every plugin task method.
// It implements context.Context and provides access to flow state and the container.
//
// This is a type alias to runtime.Execution - the actual implementation lives
// in the parent runtime package. Plugin developers should use this type
// in their task method signatures.
//
// # Available Fields
//
// ID: Unique execution identifier (UUID)
//
//	exec.ID // "550e8400-e29b-41d4-a716-446655440000"
//
// Values: Map of all flow state (previous step results, properties, etc.)
//
//	exec.Values["step1_result_data"] // Access previous step output
//	exec.Values["properties_apiKey"]  // Access flow properties
//
// Flow: Reference to the flow definition
//
//	exec.Flow.Name        // Flow name
//	exec.Flow.Properties  // Flow properties
//
// Container: Access to the plugin container (for advanced use cases)
//
//	exec.Container.GetTask("other.task") // Get other tasks
//	// Note: Prefer dependency injection over Container access
//
// # Context Implementation
//
// Execution implements context.Context, so it can be passed to functions
// expecting context:
//
//	func (p *MyPlugin) Task(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
//	    // Pass to functions expecting context.Context
//	    result, err := someAPI.Call(exec, data)
//	    return plugin.Output{"result": result}, err
//	}
//
// # Accessing Flow State
//
// Values are stored with formatted keys (dots/hyphens converted to underscores):
//
//	// In flow YAML:
//	// steps:
//	//   - id: fetch-data
//	//     ...
//	// Access in plugin:
//	previousData := exec.Values["fetch_data_result"] // Note: dash → underscore
//
// # Helper Methods
//
// AddValue(key string, value any): Add value to execution state
//
//	exec.AddValue("my_key", "my_value")
//
// Value(key any): Get value from execution state (context.Context method)
//
//	val := exec.Value("my_key")
//
// # Usage Example
//
//	func (p *HTTPPlugin) Request(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
//	    // Access flow state
//	    apiKey := exec.Values["properties_apiKey"].(string)
//
//	    // Access previous step results
//	    if userId, ok := exec.Values["auth_result_user_id"]; ok {
//	        // Use userId in request
//	    }
//
//	    // Pass as context
//	    req, err := http.NewRequestWithContext(exec, "GET", url, nil)
//
//	    // Store results (optional - framework does this automatically)
//	    exec.AddValue("request_completed", true)
//
//	    return plugin.Output{"status": 200, "body": data}, nil
//	}
type Execution = runtime.Execution

// Note: This is a type alias. The actual Execution struct is defined in
// the parent runtime package. This allows plugins to reference the type
// without importing runtime directly (they import runtime/plugin instead).
