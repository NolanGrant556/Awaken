package officerender

import (
	"context"
	"sync"
)

var sharedHostWorker = struct {
	sync.Mutex
	worker *OfficeRenderWorker
}{}

// SharedHostWorker returns one process-wide worker. The resolver runs before
// every conversion, so a user or Agent can install LibreOffice and retry in the
// same Awaken process without publishing an Awaken-owned runtime revision.
func SharedHostWorker() *OfficeRenderWorker {
	sharedHostWorker.Lock()
	defer sharedHostWorker.Unlock()
	if sharedHostWorker.worker == nil {
		sharedHostWorker.worker = NewHostRuntimeResolver().NewWorker(Config{})
	}
	return sharedHostWorker.worker
}

func Render(ctx context.Context, inputPath, outputPath, format string) (Result, error) {
	return SharedHostWorker().Render(ctx, Job{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Format:     format,
	})
}
