package common

import (
	"strconv"

	"github.com/lyft/flytepropeller/pkg/apis/flyteworkflow/v1alpha1"
	"github.com/lyft/flytepropeller/pkg/controller/executors"
	"github.com/lyft/flytepropeller/pkg/utils"
)

const maxUniqueIDLength = 20

// The UniqueId of a node is unique within a given workflow execution.
// In order to achieve that we track the lineage of the node.
// To compute the uniqueID of a node, we use the uniqueID and retry attempt of the parent node
// For nodes in level 0, there is no parent, and parentInfo is nil
func GenerateUniqueID(parentInfo executors.ImmutableParentInfo, nodeID string) (string, error) {
	var parentUniqueID v1alpha1.NodeID
	var parentRetryAttempt string

	if parentInfo != nil {
		parentUniqueID = parentInfo.GetUniqueID()
		parentRetryAttempt = strconv.Itoa(int(parentInfo.CurrentAttempt()))
	}

	return utils.FixedLengthUniqueIDForParts(maxUniqueIDLength, parentUniqueID, parentRetryAttempt, nodeID)
}

// When creating parentInfo, the unique id of parent is dependent on the unique id and the current attempt of the grand parent to track the lineage.
func CreateParentInfo(grandParentInfo executors.ImmutableParentInfo, nodeID string, parentAttempt uint32) (executors.ImmutableParentInfo, error) {
	uniqueID, err := GenerateUniqueID(grandParentInfo, nodeID)
	if err != nil {
		return nil, err
	}
	return executors.NewParentInfo(uniqueID, parentAttempt), nil

}
