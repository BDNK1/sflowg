package runtime

import (
	"fmt"
	"strings"
)

func validateExecutionResponse(execution *Execution) *FlowError {
	return validateResponse(execution, execution.State().Response())
}

func validateRecoveryResponse(execution *Execution, response *ResponseDescriptor) *FlowError {
	return validateResponse(execution, response)
}

func validateResponse(execution *Execution, response *ResponseDescriptor) *FlowError {
	contract := responseContractForExecution(execution)
	if contract.IsZero() {
		return nil
	}
	if response == nil {
		if contract.RequiresResponse {
			return responseContractError("missing_response", "flow completed without a required response")
		}
		return nil
	}
	subtype := responseSubtype(response)
	if !contract.HasSubtype(subtype) {
		return responseContractError("invalid_response", fmt.Sprintf("response.%s is not valid for entrypoint.%s", subtype, contract.EntrypointType))
	}
	return nil
}

func responseContractForExecution(execution *Execution) ResponseContract {
	if execution == nil || execution.Flow == nil {
		return ResponseContract{}
	}
	if !execution.Flow.ResponseContract.IsZero() {
		return execution.Flow.ResponseContract
	}
	return ResponseContract{}
}

func responseSubtype(response *ResponseDescriptor) string {
	if response == nil {
		return ""
	}
	if response.Subtype != "" {
		return response.Subtype
	}
	if dot := strings.LastIndex(response.HandlerName, "."); dot >= 0 && dot+1 < len(response.HandlerName) {
		return response.HandlerName[dot+1:]
	}
	return response.HandlerName
}

func responseContractError(reason string, message string) *FlowError {
	return &FlowError{
		Type:    ErrorTypePermanent,
		Code:    string(ErrorCodeRuntimeError),
		Message: message,
		Meta: map[string]any{
			"reason": reason,
		},
	}
}
