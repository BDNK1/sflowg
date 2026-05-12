package dsl

import (
	"context"

	"github.com/BDNK1/sflowg/core"
)

type compileContext struct {
	ctx                context.Context
	flow               *runtime.Flow
	container          *runtime.Container
	nodes              []runtime.FlowNode
	frameworkKeys      map[string]struct{}
	frameworkEnv       map[string]any
	frameworkStoreKeys []string
	contract           runtime.ResponseContract
	allStoreKeys       []string
	allReferenceKeys   []string
	asyncStepIndexes   map[string]int
}

func newCompileContext(ctx context.Context, flow *runtime.Flow, container *runtime.Container) *compileContext {
	frameworkKeys, frameworkEnv := collectFrameworkInfo(container)
	contract := responseContractForFlow(flow)
	frameworkEnv["response"] = buildResponseTemplateModule(contract)
	nodes := runtime.NormalizeFlowNodes(flow)
	frameworkStoreKeys := collectFrameworkStoreKeys(flow)
	allStoreKeys := visibleKeysAfterAllNodes(nodes, frameworkStoreKeys)

	return &compileContext{
		ctx:                ctx,
		flow:               flow,
		container:          container,
		nodes:              nodes,
		frameworkKeys:      frameworkKeys,
		frameworkEnv:       frameworkEnv,
		frameworkStoreKeys: frameworkStoreKeys,
		contract:           contract,
		allStoreKeys:       allStoreKeys,
		allReferenceKeys:   dedupeStrings(append(append([]string{}, allStoreKeys...), foreachBodyStepKeys(nodes)...)),
		asyncStepIndexes:   collectAsyncStepIndexes(flow),
	}
}

func (cc *compileContext) visibleKeysBeforeNode(index int) []string {
	return visibleKeysBeforeNode(cc.nodes, index, cc.frameworkStoreKeys)
}

func (cc *compileContext) reservedResultIDs() map[string]struct{} {
	return reservedResultIDs(cc.frameworkStoreKeys, cc.frameworkKeys)
}
