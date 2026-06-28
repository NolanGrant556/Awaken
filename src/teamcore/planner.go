package teamcore

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidRunPlan = errors.New("invalid run plan")

type RunPlanner struct {
	now func() time.Time
}

func NewRunPlanner() *RunPlanner {
	return &RunPlanner{
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (p *RunPlanner) Plan(team Team, leaderPrompt string) (RunPlan, error) {
	prompt := strings.TrimSpace(leaderPrompt)
	if prompt == "" {
		return RunPlan{}, fmt.Errorf("%w: leader prompt is required", ErrInvalidRunPlan)
	}

	normalized, err := NormalizeTeam(team)
	if err != nil {
		return RunPlan{}, err
	}
	teamID := normalized.ID
	if teamID == "" {
		teamID = newID("team")
	}

	leader, ok := findMember(normalized, normalized.LeaderSlotID)
	if !ok {
		return RunPlan{}, fmt.Errorf("%w: leader slot %q not found", ErrInvalidRunPlan, normalized.LeaderSlotID)
	}

	now := p.clock()
	teamRunID := newID("team_run")
	leaderRun := MemberRun{
		ID:          newID("member_run"),
		TeamRunID:   teamRunID,
		TeamID:      teamID,
		SlotID:      leader.Slot.ID,
		Role:        RoleLeader,
		MemberName:  leader.Name,
		Input:       prompt,
		Workspace:   copyWorkspace(normalized.Workspace),
		Runtime:     copyRuntimeBinding(leader.Runtime),
		Status:      MemberRunStatusAccepted,
		SessionMode: normalized.SessionMode,
		PlannedAt:   now,
	}

	teammateRuns := make([]MemberRun, 0, len(normalized.Members)-1)
	slotWork := make([]SlotWork, 0, len(normalized.Members))
	slotWork = append(slotWork, SlotWork{
		SlotID:             leader.Slot.ID,
		Role:               RoleLeader,
		PendingWakeCount:   1,
		StartingChildCount: 0,
	})

	events := []TeamEvent{
		{
			Type:      TeamEventRunAccepted,
			TeamID:    teamID,
			TeamRunID: teamRunID,
			SlotID:    leader.Slot.ID,
			Role:      RoleLeader,
			Status:    string(TeamRunStatusAccepted),
			At:        now,
		},
		{
			Type:      TeamEventMemberRunPlan,
			TeamID:    teamID,
			TeamRunID: teamRunID,
			SlotID:    leaderRun.SlotID,
			Role:      leaderRun.Role,
			Status:    string(leaderRun.Status),
			At:        now,
		},
	}

	for _, member := range normalized.Members {
		if member.Slot.ID == leader.Slot.ID {
			continue
		}
		run := MemberRun{
			ID:           newID("member_run"),
			TeamRunID:    teamRunID,
			TeamID:       teamID,
			SlotID:       member.Slot.ID,
			Role:         member.Slot.Role,
			MemberName:   member.Name,
			Input:        prompt,
			Workspace:    copyWorkspace(normalized.Workspace),
			Runtime:      copyRuntimeBinding(member.Runtime),
			ParentSlotID: leader.Slot.ID,
			ParentRunID:  leaderRun.ID,
			Status:       MemberRunStatusAccepted,
			SessionMode:  normalized.SessionMode,
			PlannedAt:    now,
		}
		teammateRuns = append(teammateRuns, run)
		slotWork = append(slotWork, SlotWork{
			SlotID:             member.Slot.ID,
			Role:               member.Slot.Role,
			PendingWakeCount:   1,
			StartingChildCount: 0,
		})
		events = append(events, TeamEvent{
			Type:      TeamEventMemberRunPlan,
			TeamID:    teamID,
			TeamRunID: teamRunID,
			SlotID:    run.SlotID,
			Role:      run.Role,
			Status:    string(run.Status),
			At:        now,
		})
	}

	teamRun := TeamRun{
		ID:                 teamRunID,
		TeamID:             teamID,
		LeaderPrompt:       prompt,
		TargetSlotID:       leader.Slot.ID,
		TargetRole:         RoleLeader,
		Status:             TeamRunStatusAccepted,
		ActiveChildCount:   0,
		PendingWakeCount:   len(slotWork),
		StartingChildCount: 0,
		SlotWork:           slotWork,
		CreatedAt:          now,
	}

	return RunPlan{
		TeamRun:      teamRun,
		LeaderRun:    leaderRun,
		TeammateRuns: teammateRuns,
		Events:       events,
	}, nil
}

func (p *RunPlanner) clock() time.Time {
	if p == nil || p.now == nil {
		return time.Now().UTC()
	}
	return p.now().UTC()
}
