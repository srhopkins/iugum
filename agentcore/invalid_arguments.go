package agentcore

// InvalidToolArguments marks a syntactic argument mistake that the model can
// correct. Only tool execution results are recoverable; policy and Authorize
// errors remain fatal. Message must be fixed guidance without private values.
type InvalidToolArguments struct{ Message string }

func (e *InvalidToolArguments) Error() string { return e.Message }
