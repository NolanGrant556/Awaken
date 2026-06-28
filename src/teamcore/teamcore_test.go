package teamcore

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestCreateValidationAndNormalization(t *testing.T) {
	registry := NewTeamRegistry()
	team := testTeam(t)
	team.ID = ""
	team.SessionMode = ""
	team.Workspace.WritableRoots = nil

	created, err := registry.Create(team)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected generated team id")
	}
	if created.LeaderSlotID != "lead" {
		t.Fatalf("expected leader slot lead, got %q", created.LeaderSlotID)
	}
	if created.SessionMode != SessionModePlan {
		t.Fatalf("expected default plan session mode, got %q", created.SessionMode)
	}
	if created.Workspace.Root != filepath.Clean(team.Workspace.Root) {
		t.Fatalf("expected cleaned workspace root, got %q", created.Workspace.Root)
	}
	if len(created.Workspace.WritableRoots) != 1 || created.Workspace.WritableRoots[0] != created.Workspace.Root {
		t.Fatalf("expected writable roots to default to workspace root, got %+v", created.Workspace.WritableRoots)
	}

	tests := []struct {
		name string
		team Team
	}{
		{
			name: "missing leader",
			team: func() Team {
				team := testTeam(t)
				team.Members[0].Slot.Role = RoleTeammate
				return team
			}(),
		},
		{
			name: "duplicate slot",
			team: func() Team {
				team := testTeam(t)
				team.Members[1].Slot.ID = "lead"
				return team
			}(),
		},
		{
			name: "relative workspace",
			team: func() Team {
				team := testTeam(t)
				team.Workspace.Root = "relative/path"
				return team
			}(),
		},
		{
			name: "missing runtime binding",
			team: func() Team {
				team := testTeam(t)
				team.Members[1].Runtime.Backend = ""
				return team
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := registry.Create(tt.team); !errors.Is(err, ErrInvalidTeam) && !errors.Is(err, ErrInvalidMember) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestMemberLifecycle(t *testing.T) {
	registry := NewTeamRegistry()
	created, err := registry.Create(testTeam(t))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	added, err := registry.AddMember(created.ID, Member{
		Slot:    Slot{ID: "writer", Role: RoleTeammate},
		Name:    "Writer",
		Runtime: RuntimeBinding{Backend: "claude-code", AgentType: "writer", Model: "sonnet"},
	})
	if err != nil {
		t.Fatalf("AddMember returned error: %v", err)
	}
	if len(added.Members) != 3 {
		t.Fatalf("expected 3 members after add, got %d", len(added.Members))
	}

	renamed, err := registry.RenameMember(created.ID, "writer", "Draft Writer")
	if err != nil {
		t.Fatalf("RenameMember returned error: %v", err)
	}
	writer, ok := findMember(renamed, "writer")
	if !ok || writer.Name != "Draft Writer" {
		t.Fatalf("expected renamed writer, got %+v", writer)
	}

	removed, err := registry.RemoveMember(created.ID, "writer")
	if err != nil {
		t.Fatalf("RemoveMember returned error: %v", err)
	}
	if _, ok := findMember(removed, "writer"); ok {
		t.Fatalf("expected writer to be removed")
	}
	if _, err := registry.RemoveMember(created.ID, "lead"); !errors.Is(err, ErrInvalidTeam) {
		t.Fatalf("expected leader removal to fail with ErrInvalidTeam, got %v", err)
	}

	snapshot, err := registry.Get(created.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	snapshot.Members[0].Name = "mutated outside"
	again, err := registry.Get(created.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if again.Members[0].Name == "mutated outside" {
		t.Fatalf("expected Get to return a defensive copy")
	}
}

func TestSessionModeUpdateAndValidation(t *testing.T) {
	registry := NewTeamRegistry()
	created, err := registry.Create(testTeam(t))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	updated, err := registry.SetSessionMode(created.ID, SessionModeAuto)
	if err != nil {
		t.Fatalf("SetSessionMode(auto) returned error: %v", err)
	}
	if updated.SessionMode != SessionModeAuto {
		t.Fatalf("expected auto session mode, got %q", updated.SessionMode)
	}

	updated, err = registry.SetSessionMode(created.ID, SessionModeManual)
	if err != nil {
		t.Fatalf("SetSessionMode(manual) returned error: %v", err)
	}
	if updated.SessionMode != SessionModeManual {
		t.Fatalf("expected manual session mode, got %q", updated.SessionMode)
	}

	if _, err := registry.SetSessionMode(created.ID, SessionMode("unsafe")); !errors.Is(err, ErrInvalidTeam) {
		t.Fatalf("expected invalid session mode to fail with ErrInvalidTeam, got %v", err)
	}
}

func TestRunPlannerBuildsLeaderAndTeammateRuns(t *testing.T) {
	registry := NewTeamRegistry()
	created, err := registry.Create(testTeam(t))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	created, err = registry.AddMember(created.ID, Member{
		Slot:    Slot{ID: "tester", Role: RoleTeammate},
		Name:    "Tester",
		Runtime: RuntimeBinding{Backend: "awaken", AgentType: "tester", Model: "gpt-test"},
	})
	if err != nil {
		t.Fatalf("AddMember returned error: %v", err)
	}

	planner := NewRunPlanner()
	plan, err := planner.Plan(created, "  ship team mode core  ")
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if plan.TeamRun.TeamID != created.ID {
		t.Fatalf("expected team run team id %q, got %q", created.ID, plan.TeamRun.TeamID)
	}
	if plan.TeamRun.TargetSlotID != created.LeaderSlotID || plan.TeamRun.TargetRole != RoleLeader {
		t.Fatalf("expected target leader slot, got %+v", plan.TeamRun)
	}
	if plan.TeamRun.Status != TeamRunStatusAccepted {
		t.Fatalf("expected accepted team run, got %q", plan.TeamRun.Status)
	}
	if plan.TeamRun.PendingWakeCount != 3 || len(plan.TeamRun.SlotWork) != 3 {
		t.Fatalf("expected 3 pending slot work entries, got run=%+v", plan.TeamRun)
	}
	if plan.LeaderRun.Role != RoleLeader || plan.LeaderRun.Input != "ship team mode core" {
		t.Fatalf("unexpected leader run: %+v", plan.LeaderRun)
	}
	if len(plan.TeammateRuns) != 2 {
		t.Fatalf("expected 2 teammate runs, got %d", len(plan.TeammateRuns))
	}
	for _, teammateRun := range plan.TeammateRuns {
		if teammateRun.ParentSlotID != plan.LeaderRun.SlotID || teammateRun.ParentRunID != plan.LeaderRun.ID {
			t.Fatalf("expected teammate run to point at leader parent, got %+v", teammateRun)
		}
		if teammateRun.Workspace.Root != created.Workspace.Root {
			t.Fatalf("expected teammate workspace %q, got %q", created.Workspace.Root, teammateRun.Workspace.Root)
		}
		if teammateRun.Runtime.Backend == "" {
			t.Fatalf("expected runtime binding on teammate run: %+v", teammateRun)
		}
	}
	if len(plan.Events) != 4 {
		t.Fatalf("expected run accepted plus 3 member plan events, got %d", len(plan.Events))
	}

	if _, err := planner.Plan(created, " "); !errors.Is(err, ErrInvalidRunPlan) {
		t.Fatalf("expected empty prompt to fail with ErrInvalidRunPlan, got %v", err)
	}
}

func TestMailboxOrdering(t *testing.T) {
	registry := NewTeamRegistry()
	team, err := registry.Create(testTeam(t))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	team, err = registry.AddMember(team.ID, Member{
		Slot:    Slot{ID: "tester", Role: RoleTeammate},
		Name:    "Tester",
		Runtime: RuntimeBinding{Backend: "awaken", AgentType: "tester"},
	})
	if err != nil {
		t.Fatalf("AddMember returned error: %v", err)
	}

	mailbox := NewMailbox()
	first, err := mailbox.SendToMember(team, "lead", "researcher", MailboxMessageDraft{
		Summary: "direct",
		Content: "start with API shape",
	})
	if err != nil {
		t.Fatalf("SendToMember returned error: %v", err)
	}
	broadcasts, err := mailbox.Broadcast(team, "lead", MailboxMessageDraft{
		Summary: "all hands",
		Content: "sync on shared workspace",
	})
	if err != nil {
		t.Fatalf("Broadcast returned error: %v", err)
	}
	last, err := mailbox.SendToMember(team, "tester", "researcher", MailboxMessageDraft{
		Summary: "test notes",
		Content: "add validation cases",
	})
	if err != nil {
		t.Fatalf("SendToMember returned error: %v", err)
	}

	if len(broadcasts) != 2 {
		t.Fatalf("expected broadcast to reach 2 recipients, got %d", len(broadcasts))
	}
	researcherInbox := mailbox.ListMessages(team.ID, "researcher")
	if len(researcherInbox) != 3 {
		t.Fatalf("expected 3 researcher messages, got %+v", researcherInbox)
	}
	if researcherInbox[0].ID != first.ID || researcherInbox[2].ID != last.ID {
		t.Fatalf("expected mailbox to preserve recipient ordering, got %+v", researcherInbox)
	}
	for i := 1; i < len(researcherInbox); i++ {
		if researcherInbox[i].Seq <= researcherInbox[i-1].Seq {
			t.Fatalf("expected increasing sequence numbers, got %+v", researcherInbox)
		}
	}
	testerInbox := mailbox.ListMessages(team.ID, "tester")
	if len(testerInbox) != 1 || !testerInbox[0].Broadcast {
		t.Fatalf("expected tester to receive one broadcast, got %+v", testerInbox)
	}

	if _, err := mailbox.SendToMember(team, "lead", "missing", MailboxMessageDraft{Content: "hello"}); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("expected missing recipient to fail with ErrMemberNotFound, got %v", err)
	}
}

func TestArchivePinUpdateListAndDelete(t *testing.T) {
	registry := NewTeamRegistry()
	first, err := registry.Create(testTeamWithID(t, "team-a"))
	if err != nil {
		t.Fatalf("Create first returned error: %v", err)
	}
	second, err := registry.Create(testTeamWithID(t, "team-b"))
	if err != nil {
		t.Fatalf("Create second returned error: %v", err)
	}

	pinned, err := registry.Pin(second.ID, true)
	if err != nil {
		t.Fatalf("Pin returned error: %v", err)
	}
	if !pinned.Pinned {
		t.Fatalf("expected team to be pinned")
	}
	archived, err := registry.Archive(second.ID, true)
	if err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}
	if !archived.Archived {
		t.Fatalf("expected team to be archived")
	}

	listed := registry.List()
	if len(listed) != 2 || listed[0].ID != second.ID {
		t.Fatalf("expected pinned team first in list, got %+v", listed)
	}

	first.Name = "Renamed Team"
	updated, err := registry.Update(first)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Name != "Renamed Team" {
		t.Fatalf("expected updated team name, got %q", updated.Name)
	}

	if err := registry.Delete(first.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := registry.Get(first.ID); !errors.Is(err, ErrTeamNotFound) {
		t.Fatalf("expected deleted team to be missing, got %v", err)
	}
}

func testTeam(t *testing.T) Team {
	t.Helper()
	return testTeamWithID(t, "team-1")
}

func testTeamWithID(t *testing.T, id string) Team {
	t.Helper()
	workspaceRoot := t.TempDir()
	return Team{
		ID:          id,
		Name:        "Core Team",
		Description: "leader teammate core",
		Workspace: SharedWorkspace{
			Root: workspaceRoot,
			Mode: WorkspaceModeShared,
		},
		SessionMode: SessionModePlan,
		Members: []Member{
			{
				Slot: Slot{ID: "lead", Role: RoleLeader},
				Name: "Leader",
				Runtime: RuntimeBinding{
					Backend:   "awaken",
					AgentID:   "leader@core",
					AgentType: "team-lead",
					Model:     "gpt-test",
				},
			},
			{
				Slot: Slot{ID: "researcher", Role: RoleTeammate},
				Name: "Researcher",
				Runtime: RuntimeBinding{
					Backend:   "claude-code",
					AgentID:   "researcher@core",
					AgentType: "researcher",
					Model:     "sonnet",
				},
			},
		},
	}
}
