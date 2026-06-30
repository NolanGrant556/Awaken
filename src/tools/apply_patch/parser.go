package applypatch

import (
	"fmt"
	"strings"
)

const (
	beginPatchMarker    = "*** Begin Patch"
	endPatchMarker      = "*** End Patch"
	environmentIDMarker = "*** Environment ID:"
	addFileMarker       = "*** Add File: "
	deleteFileMarker    = "*** Delete File: "
	updateFileMarker    = "*** Update File: "
	moveToMarker        = "*** Move to: "
	changeContextMarker = "@@ "
	emptyContextMarker  = "@@"
	endOfFileMarker     = "*** End of File"
)

func parsePatch(input string) (*patchDocument, error) {
	lines, err := normalizedPatchLines(input)
	if err != nil {
		return nil, err
	}
	if len(lines) < 3 {
		return nil, fmt.Errorf("invalid patch: patch must contain Begin Patch, at least one hunk, and End Patch")
	}
	if strings.TrimSpace(lines[0]) != beginPatchMarker {
		return nil, fmt.Errorf("invalid patch: the first line must be %q", beginPatchMarker)
	}
	if strings.TrimSpace(lines[len(lines)-1]) != endPatchMarker {
		return nil, fmt.Errorf("invalid patch: the last line must be %q", endPatchMarker)
	}

	doc := &patchDocument{}
	for i := 1; i < len(lines)-1; {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			i++
			continue
		}
		if strings.HasPrefix(trimmed, environmentIDMarker) {
			if doc.environmentID != "" {
				return nil, fmt.Errorf("invalid patch: Environment ID cannot be specified more than once")
			}
			environmentID := strings.TrimSpace(strings.TrimPrefix(trimmed, environmentIDMarker))
			if environmentID == "" {
				return nil, fmt.Errorf("invalid patch: Environment ID cannot be empty")
			}
			doc.environmentID = environmentID
			i++
			continue
		}

		var op patchOperation
		var next int
		var err error
		switch {
		case strings.HasPrefix(trimmed, addFileMarker):
			op, next, err = parseAddFile(lines, i)
		case strings.HasPrefix(trimmed, deleteFileMarker):
			op, next, err = parseDeleteFile(lines, i)
		case strings.HasPrefix(trimmed, updateFileMarker):
			op, next, err = parseUpdateFile(lines, i)
		default:
			return nil, fmt.Errorf("invalid patch hunk at line %d: %q is not a valid hunk header", i+1, trimmed)
		}
		if err != nil {
			return nil, err
		}
		doc.operations = append(doc.operations, op)
		i = next
	}

	if len(doc.operations) == 0 {
		return nil, fmt.Errorf("invalid patch: patch must contain at least one hunk")
	}
	return doc, nil
}

func normalizedPatchLines(input string) ([]string, error) {
	patch := strings.TrimSpace(strings.ReplaceAll(input, "\r\n", "\n"))
	patch = strings.ReplaceAll(patch, "\r", "\n")
	if patch == "" {
		return nil, fmt.Errorf("invalid patch: patch is empty")
	}

	lines := strings.Split(patch, "\n")
	if len(lines) >= 4 && isHeredocStart(strings.TrimSpace(lines[0])) && strings.HasSuffix(strings.TrimSpace(lines[len(lines)-1]), "EOF") {
		lines = lines[1 : len(lines)-1]
	}

	return lines, nil
}

func isHeredocStart(line string) bool {
	return line == "<<EOF" || line == "<<'EOF'" || line == "<<\"EOF\""
}

func parseAddFile(lines []string, start int) (patchOperation, int, error) {
	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), addFileMarker))
	if path == "" {
		return patchOperation{}, start, fmt.Errorf("invalid add hunk at line %d: missing path", start+1)
	}

	var builder strings.Builder
	i := start + 1
	for i < len(lines)-1 && !isPatchBoundary(lines[i]) {
		line := strings.TrimSuffix(lines[i], "\r")
		if !strings.HasPrefix(line, "+") {
			return patchOperation{}, start, fmt.Errorf("invalid add hunk at line %d: content lines must start with +", i+1)
		}
		builder.WriteString(strings.TrimPrefix(line, "+"))
		builder.WriteByte('\n')
		i++
	}
	if i == start+1 {
		return patchOperation{}, start, fmt.Errorf("invalid add hunk at line %d: missing file content", start+1)
	}

	return patchOperation{kind: patchKindAdd, path: path, content: builder.String()}, i, nil
}

func parseDeleteFile(lines []string, start int) (patchOperation, int, error) {
	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), deleteFileMarker))
	if path == "" {
		return patchOperation{}, start, fmt.Errorf("invalid delete hunk at line %d: missing path", start+1)
	}
	return patchOperation{kind: patchKindDelete, path: path}, start + 1, nil
}

func parseUpdateFile(lines []string, start int) (patchOperation, int, error) {
	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), updateFileMarker))
	if path == "" {
		return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: missing path", start+1)
	}

	op := patchOperation{kind: patchKindUpdate, path: path}
	i := start + 1
	for i < len(lines)-1 && !isPatchBoundary(lines[i]) {
		line := strings.TrimSuffix(lines[i], "\r")
		updateLine := strings.TrimRight(line, " \t")

		if strings.HasPrefix(updateLine, moveToMarker) {
			if len(op.chunks) > 0 {
				return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: Move to must appear before update lines", i+1)
			}
			if op.movePath != "" {
				return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: Move to cannot appear more than once", i+1)
			}
			movePath := strings.TrimSpace(strings.TrimPrefix(updateLine, moveToMarker))
			if movePath == "" {
				return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: Move to missing path", i+1)
			}
			op.movePath = movePath
			i++
			continue
		}

		if updateLine == endOfFileMarker {
			if len(op.chunks) == 0 || op.chunks[len(op.chunks)-1].isEmpty() {
				return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: End of File requires a non-empty update chunk", i+1)
			}
			op.chunks[len(op.chunks)-1].endOfFile = true
			i++
			continue
		}

		if updateLine == emptyContextMarker || strings.HasPrefix(updateLine, changeContextMarker) {
			if len(op.chunks) > 0 && op.chunks[len(op.chunks)-1].isEmpty() {
				return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: update chunk is empty", i+1)
			}
			var contextLine *string
			if strings.HasPrefix(updateLine, changeContextMarker) {
				value := strings.TrimPrefix(updateLine, changeContextMarker)
				contextLine = &value
			}
			op.chunks = append(op.chunks, updateChunk{changeContext: contextLine})
			i++
			continue
		}

		if len(op.chunks) == 0 {
			op.chunks = append(op.chunks, updateChunk{})
		}
		chunk := &op.chunks[len(op.chunks)-1]
		if chunk.endOfFile {
			if line == "" {
				i++
				continue
			}
			return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: lines after End of File must start a new @@ context", i+1)
		}

		switch {
		case line == "":
			chunk.oldLines = append(chunk.oldLines, "")
			chunk.newLines = append(chunk.newLines, "")
		case strings.HasPrefix(line, " "):
			text := strings.TrimPrefix(line, " ")
			chunk.oldLines = append(chunk.oldLines, text)
			chunk.newLines = append(chunk.newLines, text)
		case strings.HasPrefix(line, "+"):
			chunk.newLines = append(chunk.newLines, strings.TrimPrefix(line, "+"))
		case strings.HasPrefix(line, "-"):
			chunk.oldLines = append(chunk.oldLines, strings.TrimPrefix(line, "-"))
		default:
			return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: update lines must start with space, +, -, @@, Move to, or End of File", i+1)
		}
		i++
	}

	if len(op.chunks) == 0 {
		return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: update hunk is empty", start+1)
	}
	for idx, chunk := range op.chunks {
		if chunk.isEmpty() {
			return patchOperation{}, start, fmt.Errorf("invalid update hunk at line %d: update chunk %d is empty", start+1, idx+1)
		}
	}
	return op, i, nil
}

func (chunk updateChunk) isEmpty() bool {
	return len(chunk.oldLines) == 0 && len(chunk.newLines) == 0
}

func isPatchBoundary(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == endPatchMarker ||
		strings.HasPrefix(trimmed, addFileMarker) ||
		strings.HasPrefix(trimmed, deleteFileMarker) ||
		strings.HasPrefix(trimmed, updateFileMarker)
}
