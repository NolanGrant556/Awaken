package teamcore

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidTeam    = errors.New("invalid team")
	ErrInvalidMember  = errors.New("invalid member")
	ErrInvalidMessage = errors.New("invalid message")
)

func ValidateTeam(team Team) error {
	_, err := NormalizeTeam(team)
	return err
}

func NormalizeTeam(team Team) (Team, error) {
	team.ID = strings.TrimSpace(team.ID)
	team.Name = strings.TrimSpace(team.Name)
	team.Description = strings.TrimSpace(team.Description)
	team.LeaderSlotID = strings.TrimSpace(team.LeaderSlotID)

	if team.Name == "" {
		return Team{}, fmt.Errorf("%w: name is required", ErrInvalidTeam)
	}

	workspace, err := normalizeSharedWorkspace(team.Workspace)
	if err != nil {
		return Team{}, err
	}
	team.Workspace = workspace

	sessionMode, err := NormalizeSessionMode(team.SessionMode)
	if err != nil {
		return Team{}, err
	}
	team.SessionMode = sessionMode

	if len(team.Members) == 0 {
		return Team{}, fmt.Errorf("%w: at least one member is required", ErrInvalidTeam)
	}

	seenSlots := make(map[string]struct{}, len(team.Members))
	leaderSlotID := ""
	members := make([]Member, 0, len(team.Members))
	for _, member := range team.Members {
		normalized, err := normalizeMember(member)
		if err != nil {
			return Team{}, err
		}
		if _, exists := seenSlots[normalized.Slot.ID]; exists {
			return Team{}, fmt.Errorf("%w: duplicate slot %q", ErrInvalidTeam, normalized.Slot.ID)
		}
		seenSlots[normalized.Slot.ID] = struct{}{}
		if normalized.Slot.Role == RoleLeader {
			if leaderSlotID != "" {
				return Team{}, fmt.Errorf("%w: exactly one leader is required", ErrInvalidTeam)
			}
			leaderSlotID = normalized.Slot.ID
		}
		members = append(members, normalized)
	}
	if leaderSlotID == "" {
		return Team{}, fmt.Errorf("%w: leader member is required", ErrInvalidTeam)
	}
	if team.LeaderSlotID != "" && team.LeaderSlotID != leaderSlotID {
		return Team{}, fmt.Errorf("%w: leader_slot_id must point to the leader member", ErrInvalidTeam)
	}
	team.LeaderSlotID = leaderSlotID
	team.Members = members

	return copyTeam(team), nil
}

func NormalizeSessionMode(mode SessionMode) (SessionMode, error) {
	mode = SessionMode(strings.TrimSpace(string(mode)))
	if mode == "" {
		return SessionModePlan, nil
	}
	switch mode {
	case SessionModePlan, SessionModeManual, SessionModeAuto:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: unsupported session mode %q", ErrInvalidTeam, mode)
	}
}

func normalizeSharedWorkspace(workspace SharedWorkspace) (SharedWorkspace, error) {
	root := strings.TrimSpace(workspace.Root)
	if root == "" {
		return SharedWorkspace{}, fmt.Errorf("%w: shared workspace root is required", ErrInvalidTeam)
	}
	if !filepath.IsAbs(root) {
		return SharedWorkspace{}, fmt.Errorf("%w: shared workspace root must be an absolute path", ErrInvalidTeam)
	}
	root = filepath.Clean(root)

	mode := WorkspaceMode(strings.TrimSpace(string(workspace.Mode)))
	if mode == "" {
		mode = WorkspaceModeShared
	}
	switch mode {
	case WorkspaceModeShared, WorkspaceModeIsolated:
	default:
		return SharedWorkspace{}, fmt.Errorf("%w: unsupported workspace mode %q", ErrInvalidTeam, mode)
	}

	writableRoots := workspace.WritableRoots
	if len(writableRoots) == 0 {
		writableRoots = []string{root}
	} else {
		normalized := make([]string, 0, len(writableRoots))
		for _, writableRoot := range writableRoots {
			writableRoot = strings.TrimSpace(writableRoot)
			if writableRoot == "" {
				continue
			}
			if !filepath.IsAbs(writableRoot) {
				return SharedWorkspace{}, fmt.Errorf("%w: writable root %q must be an absolute path", ErrInvalidTeam, writableRoot)
			}
			normalized = append(normalized, filepath.Clean(writableRoot))
		}
		if len(normalized) == 0 {
			normalized = []string{root}
		}
		writableRoots = normalized
	}

	return SharedWorkspace{
		Root:          root,
		WritableRoots: writableRoots,
		Mode:          mode,
	}, nil
}

func normalizeMember(member Member) (Member, error) {
	member.Slot.ID = strings.TrimSpace(member.Slot.ID)
	member.Slot.Role = Role(strings.TrimSpace(string(member.Slot.Role)))
	member.Name = strings.TrimSpace(member.Name)
	member.DisplayName = strings.TrimSpace(member.DisplayName)

	if member.Slot.ID == "" {
		return Member{}, fmt.Errorf("%w: slot id is required", ErrInvalidMember)
	}
	switch member.Slot.Role {
	case RoleLeader, RoleTeammate:
	default:
		return Member{}, fmt.Errorf("%w: unsupported role %q", ErrInvalidMember, member.Slot.Role)
	}
	if member.Name == "" {
		return Member{}, fmt.Errorf("%w: name is required for slot %q", ErrInvalidMember, member.Slot.ID)
	}

	runtimeBinding, err := normalizeRuntimeBinding(member.Runtime)
	if err != nil {
		return Member{}, fmt.Errorf("%w: slot %q: %v", ErrInvalidMember, member.Slot.ID, err)
	}
	member.Runtime = runtimeBinding

	return copyMember(member), nil
}

func normalizeRuntimeBinding(binding RuntimeBinding) (RuntimeBinding, error) {
	binding.Backend = strings.TrimSpace(binding.Backend)
	binding.AgentID = strings.TrimSpace(binding.AgentID)
	binding.AgentType = strings.TrimSpace(binding.AgentType)
	binding.Model = strings.TrimSpace(binding.Model)
	binding.ConversationID = strings.TrimSpace(binding.ConversationID)
	if binding.Backend == "" {
		return RuntimeBinding{}, errors.New("runtime backend is required")
	}
	if len(binding.Metadata) > 0 {
		metadata := make(map[string]string, len(binding.Metadata))
		for key, value := range binding.Metadata {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			metadata[key] = strings.TrimSpace(value)
		}
		binding.Metadata = metadata
	}
	return copyRuntimeBinding(binding), nil
}

func copyTeam(team Team) Team {
	team.Workspace = copyWorkspace(team.Workspace)
	team.Members = copyMembers(team.Members)
	return team
}

func copyMembers(members []Member) []Member {
	if len(members) == 0 {
		return nil
	}
	copied := make([]Member, len(members))
	for i, member := range members {
		copied[i] = copyMember(member)
	}
	return copied
}

func copyMember(member Member) Member {
	member.Runtime = copyRuntimeBinding(member.Runtime)
	return member
}

func copyWorkspace(workspace SharedWorkspace) SharedWorkspace {
	if len(workspace.WritableRoots) > 0 {
		workspace.WritableRoots = append([]string(nil), workspace.WritableRoots...)
	}
	return workspace
}

func copyRuntimeBinding(binding RuntimeBinding) RuntimeBinding {
	if len(binding.Metadata) > 0 {
		metadata := make(map[string]string, len(binding.Metadata))
		for key, value := range binding.Metadata {
			metadata[key] = value
		}
		binding.Metadata = metadata
	}
	return binding
}
