package connectors

import "context"

// TeamConnectorSource defines the interface for a team connector directory.
// In v1 this is a noop placeholder; the real remote fetch will be implemented
// in a follow-up change.
type TeamConnectorSource interface {
	ConnectorSource

	// Subscribe returns a channel that emits events when the team directory
	// changes (connectors added, removed, or updated).
	Subscribe(ctx context.Context) (<-chan TeamEvent, error)
}

// TeamEvent is emitted by TeamConnectorSource.Subscribe.
type TeamEvent struct {
	Kind       string             // "added", "removed", "updated"
	Definition ConnectorDefinition
}

// noopTeamSource implements TeamConnectorSource with no connectors and a
// never-closing channel. Its List returns an empty slice.
type noopTeamSource struct{}

// NewNoopTeamSource returns a TeamConnectorSource that provides no connectors.
// This is the v1 default; real team directory support will replace it.
func NewNoopTeamSource() TeamConnectorSource {
	return &noopTeamSource{}
}

func (s *noopTeamSource) List(_ context.Context) ([]ConnectorDefinition, error) {
	return nil, nil
}

func (s *noopTeamSource) Origin() SourceOrigin { return SourceTeam }

func (s *noopTeamSource) Priority() int { return 2 }

func (s *noopTeamSource) Subscribe(ctx context.Context) (<-chan TeamEvent, error) {
	ch := make(chan TeamEvent)
	// Never close; consumer cancels via ctx.
	go func() {
		<-ctx.Done()
	}()
	return ch, nil
}