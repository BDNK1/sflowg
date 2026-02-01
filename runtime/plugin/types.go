package plugin

// Input is the type alias for map-based task input arguments.
//
// Plugin developers can use either the explicit type `plugin.Input`
// or `map[string]any` in their task method signatures - they're identical.
//
// # Usage in Task Methods
//
//	func (p *MyPlugin) Process(exec *Execution, args Input) (Output, error) {
//	    name := args["name"].(string)
//	    count := args["count"].(int)
//	    return Output{"result": name}, nil
//	}
//
// # Type Assertions
//
// Since Input is map[string]any, you need type assertions to extract values:
//
//	// Basic types
//	str := args["field"].(string)
//	num := args["count"].(int)
//	flag := args["enabled"].(bool)
//
//	// With safety check
//	if val, ok := args["optional"].(string); ok {
//	    // Use val
//	}
//
//	// Nested maps
//	nested := args["config"].(map[string]any)
//	value := nested["key"].(string)
//
// # Input Values Source
//
// Input values come from flow YAML step arguments:
//
//	steps:
//	  - id: my-step
//	    type: myplugin.process
//	    args:
//	      name: "John"
//	      count: 42
//	      enabled: true
//
// Or from expressions referencing previous step results:
//
//	args:
//	    user_id: ${ auth_result.user_id }
//	    api_key: ${ properties.apiKey }
type Input = map[string]any

// Output is the type alias for map-based task output results.
//
// Plugin developers can use either the explicit type `plugin.Output`
// or `map[string]any` in their task method signatures - they're identical.
//
// # Usage in Task Methods
//
//	func (p *MyPlugin) FetchUser(exec *Execution, args Input) (Output, error) {
//	    user := fetchUserFromDB(args["id"].(int))
//
//	    return Output{
//	        "user_id": user.ID,
//	        "email": user.Email,
//	        "name": user.Name,
//	    }, nil
//	}
//
// # Output Accessibility
//
// Output values are automatically stored in execution state and can be
// referenced in subsequent steps using expressions:
//
//	steps:
//	  - id: fetch-user
//	    type: users.fetch
//	    args:
//	        id: 123
//
//	  - id: send-email
//	    type: email.send
//	    args:
//	        to: ${ fetch_user_result.email }  # Access previous output
//	        name: ${ fetch_user_result.name }
//
// Note: Step IDs with hyphens are converted to underscores in expressions.
//
// # Nested Structures
//
// You can return complex nested structures:
//
//	return Output{
//	    "user": map[string]any{
//	        "id": 123,
//	        "profile": map[string]any{
//	            "name": "John",
//	            "age": 30,
//	        },
//	    },
//	    "metadata": map[string]any{
//	        "timestamp": time.Now(),
//	        "version": "v1",
//	    },
//	}, nil
//
// Access in subsequent steps:
//
//	${ step_id_result.user.profile.name }
type Output = map[string]any

// TaskExecutor is the interface that wraps task execution logic.
//
// This interface is used internally by the framework to wrap plugin task methods.
// Plugin developers typically don't implement this interface directly - the framework
// creates TaskExecutor implementations automatically when it discovers plugin methods.
//
// # Automatic Task Discovery
//
// When you define a plugin method with the correct signature:
//
//	func (p *MyPlugin) MyTask(exec *Execution, args Input) (Output, error)
//
// The framework automatically:
//  1. Discovers the method via reflection
//  2. Creates a TaskExecutor wrapper
//  3. Registers it with task name "myplugin.mytask"
//
// # Valid Task Method Signatures
//
// Phase 2.1 (map-based only):
//
//	func (p *PluginType) MethodName(exec *Execution, args map[string]any) (map[string]any, error)
//	func (p *PluginType) MethodName(exec *Execution, args Input) (Output, error)
//
// Both signatures are equivalent and automatically discovered.
//
// # Task Naming Convention
//
// Task names are derived from plugin name and method name:
//   - Plugin registration: RegisterPlugin("payment", paymentPlugin)
//   - Method: func (p *PaymentPlugin) Charge(...)
//   - Task name: "payment.charge" (lowercase)
//
// Used in flow YAML:
//
//	steps:
//	  - id: charge-card
//	    type: payment.charge  # References PaymentPlugin.Charge method
//
// # Internal Usage
//
// The framework uses TaskExecutor internally:
//
//	// In Container
//	type Container struct {
//	    tasks map[string]TaskExecutor
//	}
//
//	// Execution
//	executor := container.tasks["payment.charge"]
//	result, err := executor.Execute(execution, stepArgs)
//
// Plugin developers never interact with TaskExecutor directly.
type TaskExecutor interface {
	// Execute runs the task with the given execution context and arguments.
	//
	// Parameters:
	//   exec: Execution context with flow state and container access
	//   args: Input arguments from the flow YAML step
	//
	// Returns:
	//   Output: Result map that will be stored in execution state
	//   error: Error if task execution fails
	//
	// The framework wraps plugin task methods to implement this interface.
	Execute(exec *Execution, args Input) (Output, error)
}
