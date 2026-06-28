package teamcore

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrTeamNotFound   = errors.New("team not found")
	ErrTeamExists     = errors.New("team already exists")
	ErrMemberNotFound = errors.New("member not found")
)

type TeamRegistry struct {
	mu    sync.RWMutex
	teams map[string]Team
	now   func() time.Time
}

func NewTeamRegistry() *TeamRegistry {
	return &TeamRegistry{
		teams: make(map[string]Team),
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (r *TeamRegistry) Create(team Team) (Team, error) {
	normalized, err := NormalizeTeam(team)
	if err != nil {
		return Team{}, err
	}
	if normalized.ID == "" {
		normalized.ID = newID("team")
	}

	now := r.clock()
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = now
	}
	normalized.UpdatedAt = now
	for i := range normalized.Members {
		if normalized.Members[i].JoinedAt.IsZero() {
			normalized.Members[i].JoinedAt = now
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.teams[normalized.ID]; exists {
		return Team{}, fmt.Errorf("%w: %s", ErrTeamExists, normalized.ID)
	}
	r.teams[normalized.ID] = copyTeam(normalized)
	return copyTeam(normalized), nil
}

func (r *TeamRegistry) Update(team Team) (Team, error) {
	team.ID = strings.TrimSpace(team.ID)
	if team.ID == "" {
		return Team{}, fmt.Errorf("%w: id is required", ErrInvalidTeam)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.teams[team.ID]
	if !ok {
		return Team{}, fmt.Errorf("%w: %s", ErrTeamNotFound, team.ID)
	}

	normalized, err := NormalizeTeam(team)
	if err != nil {
		return Team{}, err
	}
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = existing.CreatedAt
	}
	normalized.UpdatedAt = r.clock()
	mergeMemberJoinTimes(&normalized, existing)

	r.teams[normalized.ID] = copyTeam(normalized)
	return copyTeam(normalized), nil
}

func (r *TeamRegistry) Delete(teamID string) error {
	teamID = strings.TrimSpace(teamID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.teams[teamID]; !ok {
		return fmt.Errorf("%w: %s", ErrTeamNotFound, teamID)
	}
	delete(r.teams, teamID)
	return nil
}

func (r *TeamRegistry) List() []Team {
	r.mu.RLock()
	defer r.mu.RUnlock()

	teams := make([]Team, 0, len(r.teams))
	for _, team := range r.teams {
		teams = append(teams, copyTeam(team))
	}
	sort.Slice(teams, func(i, j int) bool {
		if teams[i].Pinned != teams[j].Pinned {
			return teams[i].Pinned
		}
		if !teams[i].CreatedAt.Equal(teams[j].CreatedAt) {
			return teams[i].CreatedAt.Before(teams[j].CreatedAt)
		}
		return teams[i].ID < teams[j].ID
	})
	return teams
}

func (r *TeamRegistry) Get(teamID string) (Team, error) {
	teamID = strings.TrimSpace(teamID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	team, ok := r.teams[teamID]
	if !ok {
		return Team{}, fmt.Errorf("%w: %s", ErrTeamNotFound, teamID)
	}
	return copyTeam(team), nil
}

func (r *TeamRegistry) AddMember(teamID string, member Member) (Team, error) {
	teamID = strings.TrimSpace(teamID)
	r.mu.Lock()
	defer r.mu.Unlock()

	team, ok := r.teams[teamID]
	if !ok {
		return Team{}, fmt.Errorf("%w: %s", ErrTeamNotFound, teamID)
	}
	if member.JoinedAt.IsZero() {
		member.JoinedAt = r.clock()
	}
	team.Members = append(team.Members, member)
	team.UpdatedAt = r.clock()

	normalized, err := NormalizeTeam(team)
	if err != nil {
		return Team{}, err
	}
	mergeMemberJoinTimes(&normalized, team)
	r.teams[teamID] = copyTeam(normalized)
	return copyTeam(normalized), nil
}

func (r *TeamRegistry) RemoveMember(teamID, slotID string) (Team, error) {
	teamID = strings.TrimSpace(teamID)
	slotID = strings.TrimSpace(slotID)
	r.mu.Lock()
	defer r.mu.Unlock()

	team, ok := r.teams[teamID]
	if !ok {
		return Team{}, fmt.Errorf("%w: %s", ErrTeamNotFound, teamID)
	}
	index := memberIndex(team, slotID)
	if index < 0 {
		return Team{}, fmt.Errorf("%w: %s", ErrMemberNotFound, slotID)
	}
	if team.Members[index].Slot.Role == RoleLeader {
		return Team{}, fmt.Errorf("%w: leader member cannot be removed", ErrInvalidTeam)
	}
	team.Members = append(team.Members[:index], team.Members[index+1:]...)
	team.UpdatedAt = r.clock()

	normalized, err := NormalizeTeam(team)
	if err != nil {
		return Team{}, err
	}
	mergeMemberJoinTimes(&normalized, team)
	r.teams[teamID] = copyTeam(normalized)
	return copyTeam(normalized), nil
}

func (r *TeamRegistry) RenameMember(teamID, slotID, name string) (Team, error) {
	teamID = strings.TrimSpace(teamID)
	slotID = strings.TrimSpace(slotID)
	name = strings.TrimSpace(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	team, ok := r.teams[teamID]
	if !ok {
		return Team{}, fmt.Errorf("%w: %s", ErrTeamNotFound, teamID)
	}
	index := memberIndex(team, slotID)
	if index < 0 {
		return Team{}, fmt.Errorf("%w: %s", ErrMemberNotFound, slotID)
	}
	team.Members[index].Name = name
	team.UpdatedAt = r.clock()

	normalized, err := NormalizeTeam(team)
	if err != nil {
		return Team{}, err
	}
	mergeMemberJoinTimes(&normalized, team)
	r.teams[teamID] = copyTeam(normalized)
	return copyTeam(normalized), nil
}

func (r *TeamRegistry) Pin(teamID string, pinned bool) (Team, error) {
	return r.setTeamFlag(teamID, func(team *Team) {
		team.Pinned = pinned
	})
}

func (r *TeamRegistry) Archive(teamID string, archived bool) (Team, error) {
	return r.setTeamFlag(teamID, func(team *Team) {
		team.Archived = archived
	})
}

func (r *TeamRegistry) SetSessionMode(teamID string, mode SessionMode) (Team, error) {
	sessionMode, err := NormalizeSessionMode(mode)
	if err != nil {
		return Team{}, err
	}
	return r.setTeamFlag(teamID, func(team *Team) {
		team.SessionMode = sessionMode
	})
}

func (r *TeamRegistry) setTeamFlag(teamID string, mutate func(*Team)) (Team, error) {
	teamID = strings.TrimSpace(teamID)
	r.mu.Lock()
	defer r.mu.Unlock()

	team, ok := r.teams[teamID]
	if !ok {
		return Team{}, fmt.Errorf("%w: %s", ErrTeamNotFound, teamID)
	}
	mutate(&team)
	team.UpdatedAt = r.clock()
	r.teams[teamID] = copyTeam(team)
	return copyTeam(team), nil
}

func (r *TeamRegistry) clock() time.Time {
	if r.now == nil {
		return time.Now().UTC()
	}
	return r.now().UTC()
}

func memberIndex(team Team, slotID string) int {
	for i, member := range team.Members {
		if member.Slot.ID == slotID {
			return i
		}
	}
	return -1
}

func findMember(team Team, slotID string) (Member, bool) {
	for _, member := range team.Members {
		if member.Slot.ID == slotID {
			return copyMember(member), true
		}
	}
	return Member{}, false
}

func mergeMemberJoinTimes(next *Team, existing Team) {
	joinedAtBySlot := make(map[string]time.Time, len(existing.Members))
	for _, member := range existing.Members {
		if !member.JoinedAt.IsZero() {
			joinedAtBySlot[member.Slot.ID] = member.JoinedAt
		}
	}
	now := next.UpdatedAt
	for i := range next.Members {
		if !next.Members[i].JoinedAt.IsZero() {
			continue
		}
		if joinedAt, ok := joinedAtBySlot[next.Members[i].Slot.ID]; ok {
			next.Members[i].JoinedAt = joinedAt
			continue
		}
		next.Members[i].JoinedAt = now
	}
}
