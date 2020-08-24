package branch

import (
	"context"
	"fmt"
	"testing"

	"github.com/lyft/flytepropeller/pkg/controller/nodes/common"

	"github.com/lyft/flyteidl/gen/pb-go/flyteidl/core"
	mocks3 "github.com/lyft/flyteplugins/go/tasks/pluginmachinery/io/mocks"
	"github.com/lyft/flytestdlib/contextutils"
	"github.com/lyft/flytestdlib/promutils"
	"github.com/lyft/flytestdlib/promutils/labeled"
	"github.com/lyft/flytestdlib/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lyft/flytepropeller/pkg/apis/flyteworkflow/v1alpha1"
	mocks2 "github.com/lyft/flytepropeller/pkg/apis/flyteworkflow/v1alpha1/mocks"
	"github.com/lyft/flytepropeller/pkg/controller/executors"
	execMocks "github.com/lyft/flytepropeller/pkg/controller/executors/mocks"
	"github.com/lyft/flytepropeller/pkg/controller/nodes/handler"
	"github.com/lyft/flytepropeller/pkg/controller/nodes/handler/mocks"
)

type branchNodeStateHolder struct {
	s handler.BranchNodeState
}

func (t *branchNodeStateHolder) PutTaskNodeState(s handler.TaskNodeState) error {
	panic("not implemented")
}

func (t *branchNodeStateHolder) PutBranchNode(s handler.BranchNodeState) error {
	t.s = s
	return nil
}

func (t branchNodeStateHolder) PutWorkflowNodeState(s handler.WorkflowNodeState) error {
	panic("not implemented")
}

func (t branchNodeStateHolder) PutDynamicNodeState(s handler.DynamicNodeState) error {
	panic("not implemented")
}

type parentInfo struct {
}

func (parentInfo) GetUniqueID() v1alpha1.NodeID {
	return "u1"
}

func (parentInfo) CurrentAttempt() uint32 {
	return uint32(2)
}

func createNodeContext(phase v1alpha1.BranchNodePhase, childNodeID *v1alpha1.NodeID, n v1alpha1.ExecutableNode, inputs *core.LiteralMap, nl executors.NodeLookup, eCtx executors.ExecutionContext) (*mocks.NodeExecutionContext, *branchNodeStateHolder) {
	branchNodeState := handler.BranchNodeState{
		FinalizedNodeID: childNodeID,
		Phase:           phase,
	}
	s := &branchNodeStateHolder{s: branchNodeState}

	wfExecID := &core.WorkflowExecutionIdentifier{
		Project: "project",
		Domain:  "domain",
		Name:    "name",
	}

	nm := &mocks.NodeExecutionMetadata{}
	nm.OnGetAnnotations().Return(map[string]string{})
	nm.OnGetNodeExecutionID().Return(&core.NodeExecutionIdentifier{
		ExecutionId: wfExecID,
		NodeId:      n.GetID(),
	})
	nm.OnGetK8sServiceAccount().Return("service-account")
	nm.OnGetLabels().Return(map[string]string{})
	nm.OnGetNamespace().Return("namespace")
	nm.OnGetOwnerID().Return(types.NamespacedName{Namespace: "namespace", Name: "name"})
	nm.OnGetOwnerReference().Return(v1.OwnerReference{
		Kind: "sample",
		Name: "name",
	})

	ns := &mocks2.ExecutableNodeStatus{}
	ns.OnGetDataDir().Return(storage.DataReference("data-dir"))
	ns.OnGetPhase().Return(v1alpha1.NodePhaseNotYetStarted)

	ir := &mocks3.InputReader{}
	ir.OnGetMatch(mock.Anything).Return(inputs, nil)

	nCtx := &mocks.NodeExecutionContext{}
	nCtx.OnNodeExecutionMetadata().Return(nm)
	nCtx.OnNode().Return(n)
	nCtx.OnInputReader().Return(ir)
	tmpDataStore, _ := storage.NewDataStore(&storage.Config{Type: storage.TypeMemory}, promutils.NewTestScope())
	nCtx.OnDataStore().Return(tmpDataStore)
	nCtx.OnCurrentAttempt().Return(uint32(1))
	nCtx.OnMaxDatasetSizeBytes().Return(int64(1))
	nCtx.OnNodeStatus().Return(ns)

	nCtx.OnNodeID().Return("n1")
	nCtx.OnEnqueueOwnerFunc().Return(nil)

	nr := &mocks.NodeStateReader{}
	nr.OnGetBranchNode().Return(handler.BranchNodeState{
		FinalizedNodeID: childNodeID,
		Phase:           phase,
	})
	nCtx.OnNodeStateReader().Return(nr)
	nCtx.OnNodeStateWriter().Return(s)
	nCtx.OnExecutionContext().Return(eCtx)
	nCtx.OnContextualNodeLookup().Return(nl)
	return nCtx, s
}

func TestBranchHandler_RecurseDownstream(t *testing.T) {
	ctx := context.TODO()

	childNodeID := "child"
	nodeID := "n1"

	res := &v12.ResourceRequirements{}
	n := &mocks2.ExecutableNode{}
	n.OnGetResources().Return(res)
	n.OnGetID().Return(nodeID)

	expectedError := &core.ExecutionError{}
	bn := &mocks2.ExecutableNode{}
	bn.OnGetID().Return(childNodeID)

	tests := []struct {
		name            string
		ns              executors.NodeStatus
		err             error
		nodeStatus      *mocks2.ExecutableNodeStatus
		branchTakenNode v1alpha1.ExecutableNode
		isErr           bool
		expectedPhase   handler.EPhase
		childPhase      v1alpha1.NodePhase
		nl              *execMocks.NodeLookup
	}{
		{"childNodeError", executors.NodeStatusUndefined, fmt.Errorf("err"),
			nil, bn, true, handler.EPhaseUndefined, v1alpha1.NodePhaseFailed, nil},
		{"childPending", executors.NodeStatusPending, nil,
			nil, bn, false, handler.EPhaseRunning, v1alpha1.NodePhaseQueued, nil},
		{"childStillRunning", executors.NodeStatusRunning, nil,
			nil, bn, false, handler.EPhaseRunning, v1alpha1.NodePhaseRunning, nil},
		{"childFailure", executors.NodeStatusFailed(expectedError), nil,
			nil, bn, false, handler.EPhaseFailed, v1alpha1.NodePhaseFailed, nil},
		{"childComplete", executors.NodeStatusComplete, nil,
			&mocks2.ExecutableNodeStatus{}, bn, false, handler.EPhaseSuccess, v1alpha1.NodePhaseSucceeded, &execMocks.NodeLookup{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			eCtx := &execMocks.ExecutionContext{}
			eCtx.OnGetParentInfo().Return(parentInfo{})
			nCtx, _ := createNodeContext(v1alpha1.BranchNodeNotYetEvaluated, &childNodeID, n, nil, test.nl, eCtx)
			newParentInfo, _ := common.CreateParentInfo(parentInfo{}, nCtx.NodeID(), nCtx.CurrentAttempt())
			expectedExecContext := executors.NewExecutionContextWithParentInfo(nCtx.ExecutionContext(), newParentInfo)
			mockNodeExecutor := &execMocks.Node{}
			mockNodeExecutor.OnRecursiveNodeHandlerMatch(
				mock.Anything, // ctx
				mock.MatchedBy(func(e executors.ExecutionContext) bool { return assert.Equal(t, e, expectedExecContext) }),
				mock.MatchedBy(func(d executors.DAGStructure) bool {
					if assert.NotNil(t, d) {
						fList, err1 := d.FromNode("x")
						dList, err2 := d.ToNode(childNodeID)
						b := assert.NoError(t, err1)
						b = b && assert.Equal(t, fList, []v1alpha1.NodeID{})
						b = b && assert.NoError(t, err2)
						b = b && assert.Equal(t, dList, []v1alpha1.NodeID{nodeID})
						return b
					}
					return false
				}),
				mock.MatchedBy(func(lookup executors.NodeLookup) bool { return assert.Equal(t, lookup, test.nl) }),
				mock.MatchedBy(func(n v1alpha1.ExecutableNode) bool { return assert.Equal(t, n.GetID(), childNodeID) }),
			).Return(test.ns, test.err)

			childNodeStatus := &mocks2.ExecutableNodeStatus{}
			if test.nl != nil {
				childNodeStatus.OnGetDataDir().Return("child-data-dir")
				childNodeStatus.OnGetOutputDir().Return("child-output-dir")
				test.nl.OnGetNodeExecutionStatus(ctx, childNodeID).Return(childNodeStatus)
				test.nodeStatus.On("SetDataDir", storage.DataReference("child-data-dir")).Once()
				test.nodeStatus.On("SetOutputDir", storage.DataReference("child-output-dir")).Once()
			}
			branch := New(mockNodeExecutor, promutils.NewTestScope()).(*branchHandler)
			h, err := branch.recurseDownstream(ctx, nCtx, test.nodeStatus, test.branchTakenNode)
			if test.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expectedPhase, h.Info().GetPhase())
		})
	}
}

func TestBranchHandler_AbortNode(t *testing.T) {
	ctx := context.TODO()
	b1 := "b1"
	n1 := "n1"
	n2 := "n2"

	exp, _ := getComparisonExpression(1.0, core.ComparisonExpression_EQ, 1.0)
	branchNode := &v1alpha1.BranchNodeSpec{

		If: v1alpha1.IfBlock{
			Condition: v1alpha1.BooleanExpression{
				BooleanExpression: &core.BooleanExpression{
					Expr: &core.BooleanExpression_Comparison{
						Comparison: exp,
					},
				},
			},
			ThenNode: &n1,
		},
		ElseIf: []*v1alpha1.IfBlock{
			{
				Condition: v1alpha1.BooleanExpression{
					BooleanExpression: &core.BooleanExpression{
						Expr: &core.BooleanExpression_Comparison{
							Comparison: exp,
						},
					},
				},
				ThenNode: &n2,
			},
		},
	}

	n := &v1alpha1.NodeSpec{
		ID:         n2,
		BranchNode: branchNode,
	}

	w := &v1alpha1.FlyteWorkflow{
		WorkflowSpec: &v1alpha1.WorkflowSpec{
			ID: "test",
			Nodes: map[v1alpha1.NodeID]*v1alpha1.NodeSpec{
				n1: {
					ID: n1,
				},
				n2: n,
			},
		},
		Status: v1alpha1.WorkflowStatus{
			NodeStatus: map[v1alpha1.NodeID]*v1alpha1.NodeStatus{
				b1: {
					Phase: v1alpha1.NodePhaseRunning,
					BranchStatus: &v1alpha1.BranchNodeStatus{
						FinalizedNodeID: &n1,
					},
				},
			},
		},
	}
	assert.NotNil(t, w)

	t.Run("NoBranchNode", func(t *testing.T) {
		mockNodeExecutor := &execMocks.Node{}
		mockNodeExecutor.OnAbortHandlerMatch(mock.Anything,
			mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("err"))
		eCtx := &execMocks.ExecutionContext{}
		eCtx.OnGetParentInfo().Return(nil)
		nCtx, _ := createNodeContext(v1alpha1.BranchNodeError, nil, n, nil, nil, eCtx)
		branch := New(mockNodeExecutor, promutils.NewTestScope())
		err := branch.Abort(ctx, nCtx, "")
		assert.NoError(t, err)
	})

	t.Run("BranchNodeSuccess", func(t *testing.T) {
		mockNodeExecutor := &execMocks.Node{}
		nl := &execMocks.NodeLookup{}
		eCtx := &execMocks.ExecutionContext{}
		eCtx.OnGetParentInfo().Return(parentInfo{})
		nCtx, s := createNodeContext(v1alpha1.BranchNodeSuccess, &n1, n, nil, nl, eCtx)
		newParentInfo, _ := common.CreateParentInfo(parentInfo{}, nCtx.NodeID(), nCtx.CurrentAttempt())
		expectedExecContext := executors.NewExecutionContextWithParentInfo(nCtx.ExecutionContext(), newParentInfo)
		mockNodeExecutor.OnAbortHandlerMatch(mock.Anything,
			mock.MatchedBy(func(e executors.ExecutionContext) bool { return assert.Equal(t, e, expectedExecContext) }),
			mock.Anything,
			mock.Anything, mock.Anything, mock.Anything).Return(nil)
		nl.OnGetNode(*s.s.FinalizedNodeID).Return(n, true)
		branch := New(mockNodeExecutor, promutils.NewTestScope())
		err := branch.Abort(ctx, nCtx, "")
		assert.NoError(t, err)
	})
}

func TestBranchHandler_Initialize(t *testing.T) {
	ctx := context.TODO()
	mockNodeExecutor := &execMocks.Node{}
	branch := New(mockNodeExecutor, promutils.NewTestScope())
	assert.NoError(t, branch.Setup(ctx, nil))
}

// TODO incomplete test suite, add more
func TestBranchHandler_HandleNode(t *testing.T) {
	ctx := context.TODO()
	mockNodeExecutor := &execMocks.Node{}
	branch := New(mockNodeExecutor, promutils.NewTestScope())
	childNodeID := "child"
	childDatadir := v1alpha1.DataReference("test")
	w := &v1alpha1.FlyteWorkflow{
		WorkflowSpec: &v1alpha1.WorkflowSpec{
			ID: "test",
		},
		Status: v1alpha1.WorkflowStatus{
			NodeStatus: map[v1alpha1.NodeID]*v1alpha1.NodeStatus{
				childNodeID: {
					DataDir: childDatadir,
				},
			},
		},
	}
	assert.NotNil(t, w)

	_, inputs := getComparisonExpression(1, core.ComparisonExpression_NEQ, 1)

	tests := []struct {
		name          string
		node          v1alpha1.ExecutableNode
		isErr         bool
		expectedPhase handler.EPhase
	}{
		{"NoBranchNode", &v1alpha1.NodeSpec{}, false, handler.EPhaseFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := &v12.ResourceRequirements{}
			n := &mocks2.ExecutableNode{}
			n.OnGetResources().Return(res)
			n.OnGetBranchNode().Return(nil)
			n.OnGetID().Return("n1")
			eCtx := &execMocks.ExecutionContext{}
			eCtx.OnGetParentInfo().Return(nil)
			nCtx, _ := createNodeContext(v1alpha1.BranchNodeSuccess, &childNodeID, n, inputs, nil, eCtx)

			s, err := branch.Handle(ctx, nCtx)
			if test.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expectedPhase, s.Info().GetPhase())
		})
	}
}

func init() {
	labeled.SetMetricKeys(contextutils.ProjectKey, contextutils.DomainKey, contextutils.WorkflowIDKey, contextutils.TaskIDKey)
}
