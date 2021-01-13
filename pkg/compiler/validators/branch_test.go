package validators

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/lyft/flyteidl/gen/pb-go/flyteidl/core"
	"github.com/lyft/flytepropeller/pkg/compiler/common/mocks"
	compilerErrors "github.com/lyft/flytepropeller/pkg/compiler/errors"
	"github.com/stretchr/testify/assert"
)

func createBooleanOperand(val bool) *core.Operand {
	return &core.Operand{
		Val: &core.Operand_Primitive{
			Primitive: &core.Primitive{
				Value: &core.Primitive_Boolean{
					Boolean: val,
				},
			},
		},
	}
}

func Test_validateBranchInterface(t *testing.T) {
	identifier := core.Identifier{
		Name:    "taskName",
		Project: "project",
		Domain:  "domain",
	}

	taskNode := &core.TaskNode{
		Reference: &core.TaskNode_ReferenceId{
			ReferenceId: &identifier,
		},
	}

	coreN2 := &core.Node{
		Id: "n2",
		Target: &core.Node_TaskNode{
			TaskNode: taskNode,
		},
	}

	n2 := &mocks.NodeBuilder{}
	n2.OnGetId().Return("n2")
	n2.OnGetCoreNode().Return(coreN2)
	n2.OnGetTaskNode().Return(taskNode)
	n2.On("SetInterface", mock.Anything)

	task := &mocks.Task{}
	task.OnGetInterface().Return(&core.TypedInterface{})

	wf := &mocks.WorkflowBuilder{}
	wf.OnGetTask(identifier).Return(task, true)

	errs := compilerErrors.NewCompileErrors()
	wf.OnNewNodeBuilder(coreN2).Return(n2)

	t.Run("single branch", func(t *testing.T) {
		n := &mocks.NodeBuilder{}
		n.OnGetId().Return("n1")
		n.OnGetBranchNode().Return(&core.BranchNode{
			IfElse: &core.IfElseBlock{
				Case: &core.IfBlock{
					Condition: &core.BooleanExpression{
						Expr: &core.BooleanExpression_Comparison{
							Comparison: &core.ComparisonExpression{
								LeftValue:  createBooleanOperand(true),
								RightValue: createBooleanOperand(false),
							},
						},
					},
					ThenNode: coreN2,
				},
			},
		})

		_, ok := validateBranchInterface(wf, n, errs)
		assert.True(t, ok)
		if errs.HasErrors() {
			assert.NoError(t, errs)
		}
	})

	t.Run("two conditions", func(t *testing.T) {
		n := &mocks.NodeBuilder{}
		n.OnGetId().Return("n1")
		n.OnGetBranchNode().Return(&core.BranchNode{
			IfElse: &core.IfElseBlock{
				Case: &core.IfBlock{
					Condition: &core.BooleanExpression{
						Expr: &core.BooleanExpression_Comparison{
							Comparison: &core.ComparisonExpression{
								LeftValue:  createBooleanOperand(true),
								RightValue: createBooleanOperand(false),
							},
						},
					},
					ThenNode: coreN2,
				},
				Other: []*core.IfBlock{
					{
						Condition: &core.BooleanExpression{
							Expr: &core.BooleanExpression_Comparison{
								Comparison: &core.ComparisonExpression{
									LeftValue:  createBooleanOperand(true),
									RightValue: createBooleanOperand(false),
								},
							},
						},
						ThenNode: coreN2,
					},
				},
			},
		})

		_, ok := validateBranchInterface(wf, n, errs)
		assert.True(t, ok)
		if errs.HasErrors() {
			assert.NoError(t, errs)
		}
	})
}
