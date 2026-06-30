package applypatch

type patchKind string

const (
	patchKindAdd    patchKind = "add"
	patchKindDelete patchKind = "delete"
	patchKindUpdate patchKind = "update"
)

type patchDocument struct {
	operations    []patchOperation
	environmentID string
}

type patchOperation struct {
	kind     patchKind
	path     string
	movePath string
	content  string
	chunks   []updateChunk
}

func (op patchOperation) summaryPath() string {
	if op.kind == patchKindUpdate && op.movePath != "" {
		return op.movePath
	}
	return op.path
}

type updateChunk struct {
	changeContext *string
	oldLines      []string
	newLines      []string
	endOfFile     bool
}
