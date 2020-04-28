package executors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lyft/flytepropeller/pkg/apis/flyteworkflow/v1alpha1"
	"github.com/lyft/flytepropeller/pkg/apis/flyteworkflow/v1alpha1/mocks"
)

type ng struct {
	v1alpha1.NodeGetter
}

type nsg struct {
	v1alpha1.NodeStatusGetter
}

func TestNewNodeLookup(t *testing.T) {
	n := ng{}
	ns := nsg{}
	nl := NewNodeLookup(n, ns)
	assert.NotNil(t, nl)
	typed := nl.(contextualNodeLookup)
	assert.Equal(t, n, typed.NodeGetter)
	assert.Equal(t, ns, typed.NodeStatusGetter)
}

func TestNewTestNodeLookup(t *testing.T) {
	n := &mocks.ExecutableNode{}
	ns := &mocks.ExecutableNodeStatus{}
	nl := NewTestNodeLookup(map[string]v1alpha1.ExecutableNode{"n1": n}, map[string]v1alpha1.ExecutableNodeStatus{"n1": ns})
	assert.NotNil(t, nl)
	r, ok := nl.GetNode("n1")
	assert.True(t, ok)
	assert.Equal(t, n, r)
	assert.Equal(t, ns, nl.GetNodeExecutionStatus(context.TODO(), "n1"))


	_, ok = nl.GetNode("n")
	assert.False(t, ok)
	assert.NotEqual(t, ns, nl.GetNodeExecutionStatus(context.TODO(), "n"))
}
