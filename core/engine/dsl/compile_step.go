package dsl

import (
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/core"
)

func (c *Compiler) compileExpression(
	cc *compileContext,
	expr string,
	knownStoreKeys []string,
) ([]string, runtime.ExpressionProgram, error) {
	if strings.TrimSpace(expr) == "" {
		return nil, nil, nil
	}
	storeKeys, err := cc.extractStoreKeys(expr, knownStoreKeys)
	if err != nil {
		return nil, nil, err
	}
	templateEnv := buildStepTemplateEnv(storeKeys, cc.frameworkEnv)
	code, err := c.interpreter.Compile(cc.ctx, expr, templateEnv)
	if err != nil {
		return nil, nil, err
	}
	return storeKeys, code, nil
}

func (c *Compiler) compileStep(
	cc *compileContext,
	step *runtime.Step,
	index int,
	knownStoreKeys []string,
	inParallel bool,
	peerIDs map[string]struct{},
) error {
	return c.compileStepWithAsyncIndexes(cc, step, index, knownStoreKeys, cc.asyncStepIndexes, inParallel, peerIDs)
}

func (c *Compiler) compileLocalStep(
	cc *compileContext,
	step *runtime.Step,
	index int,
	knownStoreKeys []string,
	localAsyncIndexes map[string]int,
) error {
	return c.compileStepWithAsyncIndexes(cc, step, index, knownStoreKeys, localAsyncIndexes, false, nil)
}

func (c *Compiler) compileStepWithAsyncIndexes(
	cc *compileContext,
	step *runtime.Step,
	index int,
	knownStoreKeys []string,
	asyncStepIndexes map[string]int,
	inParallel bool,
	peerIDs map[string]struct{},
) error {
	if hasReservedInternalPrefix(step.ID) {
		return fmt.Errorf("step ID %q uses reserved internal prefix", step.ID)
	}
	if inParallel && step.CompensateBody != "" {
		return fmt.Errorf("parallel branch %q cannot have compensate block", step.ID)
	}
	if err := validateNoUserNext(step.ID, "body", step.Body, step.AllowsNext); err != nil {
		return err
	}
	if err := validateNoUserNext(step.ID, "fallback", step.FallbackBody, false); err != nil {
		return err
	}
	if inParallel {
		if err := rejectResponseCallsInStep(step); err != nil {
			return err
		}
	}

	storeKeys, compiled, err := c.compileBody(cc, step.Body, knownStoreKeys)
	if err != nil {
		return fmt.Errorf("compile step %s: %w", step.ID, err)
	}
	step.StoreKeys = storeKeys
	step.Compiled = compiled
	if err := validateNoForwardAsyncRefs(step.ID, index, "body", storeKeys, asyncStepIndexes); err != nil {
		return err
	}
	if err := validateNoPeerRefs(step.ID, "body", storeKeys, peerIDs); err != nil {
		return err
	}

	fallbackStoreKeys, fallbackCompiled, err := c.compileBody(cc, step.FallbackBody, knownStoreKeys)
	if err != nil {
		return fmt.Errorf("compile fallback for step %s: %w", step.ID, err)
	}
	step.FallbackStoreKeys = fallbackStoreKeys
	step.FallbackCompiled = fallbackCompiled
	if err := validateNoForwardAsyncRefs(step.ID, index, "fallback", fallbackStoreKeys, asyncStepIndexes); err != nil {
		return err
	}
	if err := validateNoPeerRefs(step.ID, "fallback", fallbackStoreKeys, peerIDs); err != nil {
		return err
	}

	compensateStoreKeys, compensateCompiled, err := c.compileBody(cc, step.CompensateBody, append(append([]string{}, knownStoreKeys...), step.ID))
	if err != nil {
		return fmt.Errorf("compile compensation for step %s: %w", step.ID, err)
	}
	step.CompensateCompiled = compensateCompiled
	if err := validateNoForwardAsyncRefs(step.ID, index, "compensate", compensateStoreKeys, asyncStepIndexes); err != nil {
		return err
	}

	conditionStoreKeys, err := cc.extractStoreKeys(step.Condition, knownStoreKeys)
	if err != nil {
		return fmt.Errorf("compile condition for step %s: %w", step.ID, err)
	}
	if err := validateNoForwardAsyncRefs(step.ID, index, "condition", conditionStoreKeys, asyncStepIndexes); err != nil {
		return err
	}
	if err := validateNoPeerRefs(step.ID, "condition", conditionStoreKeys, peerIDs); err != nil {
		return err
	}

	var retryStoreKeys []string
	if step.Retry != nil {
		retryStoreKeys, err = cc.extractStoreKeys(step.Retry.When, knownStoreKeys)
		if err != nil {
			return fmt.Errorf("compile retry for step %s: %w", step.ID, err)
		}
		if err := validateNoForwardAsyncRefs(step.ID, index, "retry", retryStoreKeys, asyncStepIndexes); err != nil {
			return err
		}
		if err := validateNoPeerRefs(step.ID, "retry", retryStoreKeys, peerIDs); err != nil {
			return err
		}
	}

	step.AsyncDeps = intersectAsyncDeps(asyncStepIndexes, storeKeys, fallbackStoreKeys, compensateStoreKeys, conditionStoreKeys, retryStoreKeys)
	return nil
}

func (c *Compiler) compileBody(
	cc *compileContext,
	body string,
	knownStoreKeys []string,
) ([]string, any, error) {
	if strings.TrimSpace(body) == "" {
		return nil, nil, nil
	}

	if err := ValidateResponseCalls(body, cc.contract); err != nil {
		return nil, nil, err
	}

	storeKeys, err := ExtractStoreKeys(body, knownStoreKeys, cc.frameworkKeys)
	if err != nil {
		return nil, nil, err
	}
	if storeKeys == nil {
		storeKeys = []string{}
	}

	templateEnv := buildStepTemplateEnv(storeKeys, cc.frameworkEnv)
	code, err := c.interpreter.Compile(cc.ctx, body, templateEnv)
	if err != nil {
		return nil, nil, err
	}

	return storeKeys, code, nil
}

func (cc *compileContext) extractStoreKeys(source string, knownStoreKeys []string) ([]string, error) {
	storeKeys, err := ExtractStoreKeys(source, knownStoreKeys, cc.frameworkKeys)
	if err != nil {
		return nil, err
	}
	if storeKeys == nil {
		return []string{}, nil
	}
	return storeKeys, nil
}
