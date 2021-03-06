package task

import (
	"context"

	"github.com/lyft/flyteidl/gen/pb-go/flyteidl/core"
	"github.com/lyft/flyteplugins/go/tasks/pluginmachinery/catalog"
	pluginCore "github.com/lyft/flyteplugins/go/tasks/pluginmachinery/core"
	"github.com/lyft/flyteplugins/go/tasks/pluginmachinery/io"
	"github.com/lyft/flytestdlib/logger"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lyft/flytepropeller/pkg/apis/flyteworkflow/v1alpha1"
	errors2 "github.com/lyft/flytepropeller/pkg/controller/nodes/errors"
)

func (t *Handler) CheckCatalogCache(ctx context.Context, tr pluginCore.TaskReader, inputReader io.InputReader, outputWriter io.OutputWriter) (bool, error) {
	tk, err := tr.Read(ctx)
	if err != nil {
		logger.Errorf(ctx, "Failed to read TaskTemplate, error :%s", err.Error())
		return false, err
	}
	if tk.Metadata.Discoverable {
		key := catalog.Key{
			Identifier:     *tk.Id,
			CacheVersion:   tk.Metadata.DiscoveryVersion,
			TypedInterface: *tk.Interface,
			InputReader:    inputReader,
		}

		if resp, err := t.catalog.Get(ctx, key); err != nil {
			causeErr := errors.Cause(err)
			if taskStatus, ok := status.FromError(causeErr); ok && taskStatus.Code() == codes.NotFound {
				t.metrics.catalogMissCount.Inc(ctx)
				logger.Infof(ctx, "Artifact not found in Catalog. Executing Task.")
				return false, nil
			}

			t.metrics.catalogGetFailureCount.Inc(ctx)
			logger.Errorf(ctx, "Catalog memoization check failed. err: %v", err.Error())
			return false, errors.Wrapf(err, "Failed to check Catalog for previous results")
		} else if resp != nil {
			t.metrics.catalogHitCount.Inc(ctx)
			if iface := tk.Interface; iface != nil && iface.Outputs != nil && len(iface.Outputs.Variables) > 0 {
				if err := outputWriter.Put(ctx, resp); err != nil {
					logger.Errorf(ctx, "failed to write data to Storage, err: %v", err.Error())
					return false, errors.Wrapf(err, "failed to copy cached results for task.")
				}
			}
			// SetCached.
			return true, nil
		} else {
			// Nil response and Nil error
			t.metrics.catalogGetFailureCount.Inc(ctx)
			return false, errors.Wrapf(err, "Nil catalog response. Failed to check Catalog for previous results")
		}
	}
	return false, nil
}

func (t *Handler) ValidateOutputAndCacheAdd(ctx context.Context, nodeID v1alpha1.NodeID, i io.InputReader, r io.OutputReader,
	outputCommitter io.OutputWriter, tr pluginCore.TaskReader, m catalog.Metadata) (*io.ExecutionError, error) {

	tk, err := tr.Read(ctx)
	if err != nil {
		logger.Errorf(ctx, "Failed to read TaskTemplate, error :%s", err.Error())
		return nil, err
	}

	iface := tk.Interface
	outputsDeclared := iface != nil && iface.Outputs != nil && len(iface.Outputs.Variables) > 0

	if r == nil {
		if outputsDeclared {
			// Whack! plugin did not return any outputs for this task
			return &io.ExecutionError{
				ExecutionError: &core.ExecutionError{
					Code:    "OutputsNotGenerated",
					Message: "Output Reader was nil. Plugin/Platform problem.",
				},
				IsRecoverable: true,
			}, nil
		}
		return nil, nil
	}
	// Reader exists, we can check for error, even if this task may not have any outputs declared
	y, err := r.IsError(ctx)
	if err != nil {
		return nil, err
	}
	if y {
		taskErr, err := r.ReadError(ctx)
		if err != nil {
			return nil, err
		}

		if taskErr.ExecutionError == nil {
			taskErr.ExecutionError = &core.ExecutionError{Kind: core.ExecutionError_UNKNOWN, Code: "Unknown", Message: "Unknown"}
		}
		// Errors can be arbitrary long since they are written by containers/potentially 3rd party plugins. This ensures
		// the error message length will never be big enough to cause write failures to Etcd. or spam Admin DB with huge
		// objects.
		taskErr.Message = trimErrorMessage(taskErr.Message, t.cfg.MaxErrorMessageLength)
		return &taskErr, nil
	}

	// Do this if we have outputs declared for the Handler interface!
	if outputsDeclared {
		ok, err := r.Exists(ctx)
		if err != nil {
			logger.Errorf(ctx, "Failed to check if the output file exists. Error: %s", err.Error())
			return nil, err
		}

		if !ok {
			// Does not exist
			return &io.ExecutionError{
				ExecutionError: &core.ExecutionError{
					Code:    "OutputsNotFound",
					Message: "Outputs not generated by task execution",
				},
				IsRecoverable: true,
			}, nil
		}

		if !r.IsFile(ctx) {
			// Read output and write to file
			// No need to check for Execution Error here as we have done so above this block.
			err = outputCommitter.Put(ctx, r)
			if err != nil {
				logger.Errorf(ctx, "Failed to commit output to remote location. Error: %v", err)
				return nil, err
			}
		}

		// ignores discovery write failures
		if tk.Metadata.Discoverable {
			p, err := t.ResolvePlugin(ctx, tk.Type)
			if err != nil {
				return nil, errors2.Wrapf(errors2.UnsupportedTaskTypeError, nodeID, err, "unable to resolve plugin")
			}

			writeToCatalog := !p.GetProperties().DisableNodeLevelCaching
			if !writeToCatalog {
				logger.Debug(ctx, "Node level caching is disabled. Skipping catalog write.")
				return nil, nil
			}

			cacheVersion := "0"
			if tk.Metadata != nil {
				cacheVersion = tk.Metadata.DiscoveryVersion
			}

			key := catalog.Key{
				Identifier:     *tk.Id,
				CacheVersion:   cacheVersion,
				TypedInterface: *tk.Interface,
				InputReader:    i,
			}
			if err2 := t.catalog.Put(ctx, key, r, m); err2 != nil {
				t.metrics.catalogPutFailureCount.Inc(ctx)
				logger.Errorf(ctx, "Failed to write results to catalog for Task [%v]. Error: %v", tk.GetId(), err2)
			} else {
				t.metrics.catalogPutSuccessCount.Inc(ctx)
				logger.Debugf(ctx, "Successfully cached results to catalog - Task [%v]", tk.GetId())
			}
		}
	}

	return nil, nil
}
