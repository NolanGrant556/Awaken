package runtime

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

const maxMediaReferenceGroups = 8

type mediaArtifactCandidate struct {
	reference tool.ScopedMediaArtifact
	order     int
}

type mediaReferenceManifestItem struct {
	DisplayIndex int    `json:"display_index"`
	ArtifactID   string `json:"artifact_id"`
	GroupID      string `json:"group_id,omitempty"`
	GroupIndex   int    `json:"group_index,omitempty"`
}

type mediaReferenceManifestGroup struct {
	RunID string                       `json:"run_id"`
	Kind  string                       `json:"kind"`
	Items []mediaReferenceManifestItem `json:"items"`
}

func mediaArtifactReferencesFromTranscript(transcript ThreadTranscript) []tool.ScopedMediaArtifact {
	var references []tool.ScopedMediaArtifact
	for _, transcriptRun := range transcript.Runs {
		byPath := map[string]mediaArtifactCandidate{}
		var pathOrder []string
		for _, event := range transcriptRun.Events {
			artifact := event.AgentEvent
			if artifact.Type != agentcore.ArtifactCreatedEventType && artifact.Type != agentcore.ArtifactUpdatedEventType {
				continue
			}
			path := strings.TrimSpace(artifact.ArtifactPath)
			if path == "" {
				continue
			}
			key := strings.ToLower(filepath.ToSlash(path))
			if _, exists := byPath[key]; !exists {
				pathOrder = append(pathOrder, key)
			}
			byPath[key] = mediaArtifactCandidate{
				order: len(pathOrder) - 1,
				reference: tool.ScopedMediaArtifact{
					ID: artifact.ArtifactID, RunID: transcriptRun.Run.RunID,
					GroupID: artifact.ArtifactGroupID, GroupIndex: artifact.ArtifactGroupIndex,
					Kind: strings.ToLower(strings.TrimSpace(artifact.ArtifactKind)), Path: path,
					Role: artifact.ArtifactRole, Status: strings.ToLower(strings.TrimSpace(string(artifact.ArtifactStatus))),
				},
			}
		}

		groupPosition := map[string]int{}
		var candidates []mediaArtifactCandidate
		for order, key := range pathOrder {
			candidate := byPath[key]
			candidate.order = order
			ref := candidate.reference
			if ref.ID == "" || ref.Role != tool.ArtifactRoleDeliverable || ref.Status != "ready" || !supportedMediaArtifact(ref.Kind, ref.Path) {
				continue
			}
			if ref.GroupID != "" {
				if _, exists := groupPosition[ref.GroupID]; !exists {
					groupPosition[ref.GroupID] = order
				}
			}
			candidates = append(candidates, candidate)
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			left, right := candidates[i], candidates[j]
			leftPosition, rightPosition := left.order, right.order
			if left.reference.GroupID != "" {
				leftPosition = groupPosition[left.reference.GroupID]
			}
			if right.reference.GroupID != "" {
				rightPosition = groupPosition[right.reference.GroupID]
			}
			if leftPosition != rightPosition {
				return leftPosition < rightPosition
			}
			if left.reference.GroupID != "" && left.reference.GroupID == right.reference.GroupID &&
				left.reference.GroupIndex > 0 && right.reference.GroupIndex > 0 &&
				left.reference.GroupIndex != right.reference.GroupIndex {
				return left.reference.GroupIndex < right.reference.GroupIndex
			}
			return left.order < right.order
		})
		kindIndex := map[string]int{}
		for _, candidate := range candidates {
			ref := candidate.reference
			kindIndex[ref.Kind]++
			ref.DisplayIndex = kindIndex[ref.Kind]
			references = append(references, ref)
		}
	}
	return references
}

func supportedMediaArtifact(kind, path string) bool {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image":
		return extension == ".png" || extension == ".jpg" || extension == ".jpeg" || extension == ".webp"
	case "video":
		return extension == ".mp4"
	default:
		return false
	}
}

func effectiveInputForMediaArtifactReferences(input string, references []tool.ScopedMediaArtifact) string {
	groups := mediaReferenceManifestGroups(references)
	if len(groups) == 0 {
		return input
	}
	payload, err := json.Marshal(map[string]any{
		"ordinal_rule":            "用户说第N张图片或第N个视频时，N只表示最新相关媒体组在Awaken界面中的display_index；不得使用生成提示词编号、group_index或文件名猜测。调用工具时使用对应artifact_id。",
		"groups_oldest_to_newest": groups,
	})
	if err != nil {
		return input
	}
	return "<awaken_host_media_references>\n" + string(payload) + "\n</awaken_host_media_references>\n" + input
}

func mediaReferenceManifestGroups(references []tool.ScopedMediaArtifact) []mediaReferenceManifestGroup {
	var groups []mediaReferenceManifestGroup
	groupIndex := map[string]int{}
	for _, ref := range references {
		key := ref.RunID + "\x00" + ref.Kind
		index, exists := groupIndex[key]
		if !exists {
			index = len(groups)
			groupIndex[key] = index
			groups = append(groups, mediaReferenceManifestGroup{RunID: ref.RunID, Kind: ref.Kind})
		}
		groups[index].Items = append(groups[index].Items, mediaReferenceManifestItem{
			DisplayIndex: ref.DisplayIndex, ArtifactID: ref.ID,
			GroupID: ref.GroupID, GroupIndex: ref.GroupIndex,
		})
	}
	if len(groups) > maxMediaReferenceGroups {
		groups = groups[len(groups)-maxMediaReferenceGroups:]
	}
	return groups
}
