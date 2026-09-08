package runtime

import (
	"reflect"
	"testing"
	"time"
)

func TestThreadListOrderAcrossStoreAndLiveThreads(t *testing.T) {
	setFileStoreTestRoots(t)
	store := newThreadStore().(*FileDesktopStore)
	base := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir()}
	for _, test := range []struct {
		view ThreadListView
		want []string
	}{
		{ThreadListViewActive, []string{"thread_d", "thread_c", "thread_a", "thread_b", "thread_e"}},
		{ThreadListViewAll, []string{"thread_d", "thread_c", "thread_a", "thread_b", "thread_e"}},
		{ThreadListViewArchived, []string{"thread_e", "thread_d", "thread_c", "thread_a", "thread_b"}},
	} {
		t.Run(string(test.view), func(t *testing.T) {
			service := &RunService{threadStore: store, threads: map[string]*Thread{}}
			for _, fixture := range []struct {
				id                         string
				created, updated, archived time.Duration
			}{
				{"thread_b", 0, time.Hour, time.Hour},
				{"thread_e", 0, 0, 2 * time.Hour},
				{"thread_a", 0, time.Hour, time.Hour},
				{"thread_d", 0, 2 * time.Hour, time.Hour},
				{"thread_c", time.Minute, time.Hour, time.Hour},
			} {
				snapshot := ThreadSnapshot{
					ThreadID: fixture.id, Label: fixture.id, Workspace: workspace,
					CreatedAt: base.Add(fixture.created), UpdatedAt: base.Add(fixture.updated),
				}
				if err := store.CreateThread(snapshot, "", ""); err != nil {
					t.Fatal(err)
				}
				snapshot, err := store.SetThreadArchived(fixture.id, test.view == ThreadListViewArchived, base.Add(fixture.archived))
				if err != nil {
					t.Fatal(err)
				}
				// Keep some threads in memory to exercise merging and deduplication.
				if fixture.id == "thread_a" || fixture.id == "thread_c" {
					service.threads[fixture.id] = &Thread{
						threadID: fixture.id, label: "live title", workspace: workspace,
						createdAt: snapshot.CreatedAt, updatedAt: snapshot.UpdatedAt,
						archived: snapshot.Archived, archivedAt: snapshot.ArchivedAt,
					}
				}
			}
			durable, err := store.ListThreads(test.view)
			if err != nil {
				t.Fatal(err)
			}
			for source, threads := range map[string][]ThreadSnapshot{"store": durable, "service": service.ListThreads(test.view)} {
				ids := make([]string, 0, len(threads))
				for _, thread := range threads {
					ids = append(ids, thread.ThreadID)
					if source == "service" && service.threads[thread.ThreadID] != nil && thread.Label != "live title" {
						t.Fatal("durable metadata replaced the live thread")
					}
				}
				if !reflect.DeepEqual(ids, test.want) {
					t.Fatalf("%s order=%v; want %v", source, ids, test.want)
				}
			}
		})
	}
}
