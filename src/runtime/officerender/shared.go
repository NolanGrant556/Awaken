package officerender

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
)

var sharedManagedWorkers = struct {
	sync.Mutex
	byRoot map[string]*OfficeRenderWorker
}{byRoot: make(map[string]*OfficeRenderWorker)}

// SharedManagedWorker returns the process-wide worker for the currently
// configured primary-runtime root. Artifact producers in different packages
// therefore share the same semaphore, singleflight map, and attestation cache.
func SharedManagedWorker() (*OfficeRenderWorker, error) {
	root, err := DefaultManagedRuntimeRoot()
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	sharedManagedWorkers.Lock()
	defer sharedManagedWorkers.Unlock()
	if worker := sharedManagedWorkers.byRoot[root]; worker != nil {
		return worker, nil
	}
	resolver, err := NewManagedRuntimeResolver(root)
	if err != nil {
		return nil, err
	}
	worker := resolver.NewWorker(Config{})
	sharedManagedWorkers.byRoot[root] = worker
	return worker, nil
}

// RenderManaged converts through the process-wide managed LibreOffice worker.
// The active executable is resolved internally; callers provide only artifact
// paths and the target format.
func RenderManaged(ctx context.Context, inputPath, outputPath, format string) (Result, error) {
	worker, err := SharedManagedWorker()
	if err != nil {
		return Result{}, fmt.Errorf("resolve shared managed office worker: %w", err)
	}
	return worker.Render(ctx, Job{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Format:     format,
	})
}
