package configlayers

import "sort"

const RedactedValue = "<redacted>"

type ConfigKind string

const (
	BehaviorConfig ConfigKind = "behavior"
	SecretConfig   ConfigKind = "secret"
)

type Source struct {
	Name string
	Path string
}

type Entry struct {
	Value string
	Kind  ConfigKind
}

func BehaviorValue(value string) Entry {
	return Entry{
		Value: value,
		Kind:  BehaviorConfig,
	}
}

func SecretValue(value string) Entry {
	return Entry{
		Value: value,
		Kind:  SecretConfig,
	}
}

func (e Entry) IsSecret() bool {
	return normalizeKind(e.Kind) == SecretConfig
}

type Layer struct {
	Name       string
	Source     Source
	Precedence int

	values map[string]Entry
}

func NewLayer(name string, precedence int, source Source, values map[string]Entry) Layer {
	return Layer{
		Name:       name,
		Source:     source,
		Precedence: precedence,
		values:     cloneEntries(values),
	}
}

func NewBehaviorLayer(name string, precedence int, source Source, values map[string]string) Layer {
	return NewLayer(name, precedence, source, entriesFromStrings(values, BehaviorConfig))
}

func NewSecretLayer(name string, precedence int, source Source, values map[string]string) Layer {
	return NewLayer(name, precedence, source, entriesFromStrings(values, SecretConfig))
}

func (l Layer) Values() map[string]Entry {
	return cloneEntries(l.values)
}

type ValueSource struct {
	Key        string
	Value      string
	Kind       ConfigKind
	LayerName  string
	Source     Source
	Precedence int
}

func (s ValueSource) IsSecret() bool {
	return normalizeKind(s.Kind) == SecretConfig
}

func (s ValueSource) RedactedValue() string {
	if s.IsSecret() {
		return RedactedValue
	}
	return s.Value
}

func (s ValueSource) Safe() SafeValueSource {
	return SafeValueSource{
		Key:        s.Key,
		Value:      s.RedactedValue(),
		Kind:       normalizeKind(s.Kind),
		LayerName:  s.LayerName,
		Source:     s.Source,
		Precedence: s.Precedence,
	}
}

type ResolvedValue struct {
	Key        string
	Value      string
	Kind       ConfigKind
	LayerName  string
	Source     Source
	Precedence int
	Overridden []ValueSource
}

func (v ResolvedValue) IsSecret() bool {
	return normalizeKind(v.Kind) == SecretConfig
}

func (v ResolvedValue) RedactedValue() string {
	if v.IsSecret() {
		return RedactedValue
	}
	return v.Value
}

func (v ResolvedValue) Safe() SafeResolvedValue {
	overridden := make([]SafeValueSource, 0, len(v.Overridden))
	for _, source := range v.Overridden {
		overridden = append(overridden, source.Safe())
	}
	return SafeResolvedValue{
		Key:        v.Key,
		Value:      v.RedactedValue(),
		Kind:       normalizeKind(v.Kind),
		LayerName:  v.LayerName,
		Source:     v.Source,
		Precedence: v.Precedence,
		Overridden: overridden,
	}
}

type SafeValueSource struct {
	Key        string
	Value      string
	Kind       ConfigKind
	LayerName  string
	Source     Source
	Precedence int
}

type SafeResolvedValue struct {
	Key        string
	Value      string
	Kind       ConfigKind
	LayerName  string
	Source     Source
	Precedence int
	Overridden []SafeValueSource
}

type Resolution struct {
	values map[string]ResolvedValue
}

func Resolve(layers ...Layer) Resolution {
	ordered := make([]Layer, len(layers))
	copy(ordered, layers)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Precedence < ordered[j].Precedence
	})

	values := make(map[string]ResolvedValue)
	for _, layer := range ordered {
		keys := sortedEntryKeys(layer.values)
		for _, key := range keys {
			entry := layer.values[key]
			next := ResolvedValue{
				Key:        key,
				Value:      entry.Value,
				Kind:       normalizeKind(entry.Kind),
				LayerName:  layer.Name,
				Source:     layer.Source,
				Precedence: layer.Precedence,
			}
			if previous, ok := values[key]; ok {
				next.Overridden = append([]ValueSource{previous.source()}, previous.Overridden...)
				if previous.IsSecret() {
					next.Kind = SecretConfig
				}
			}
			values[key] = next
		}
	}

	return Resolution{values: cloneResolvedValues(values)}
}

func (r Resolution) Get(key string) (ResolvedValue, bool) {
	value, ok := r.values[key]
	if !ok {
		return ResolvedValue{}, false
	}
	return cloneResolvedValue(value), true
}

func (r Resolution) Keys() []string {
	keys := make([]string, 0, len(r.values))
	for key := range r.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (r Resolution) RedactedSnapshot() map[string]string {
	snapshot := make(map[string]string, len(r.values))
	for key, value := range r.values {
		snapshot[key] = value.RedactedValue()
	}
	return snapshot
}

func (r Resolution) SafeSnapshot() map[string]SafeResolvedValue {
	snapshot := make(map[string]SafeResolvedValue, len(r.values))
	for key, value := range r.values {
		snapshot[key] = value.Safe()
	}
	return snapshot
}

func (v ResolvedValue) source() ValueSource {
	return ValueSource{
		Key:        v.Key,
		Value:      v.Value,
		Kind:       normalizeKind(v.Kind),
		LayerName:  v.LayerName,
		Source:     v.Source,
		Precedence: v.Precedence,
	}
}

func entriesFromStrings(values map[string]string, kind ConfigKind) map[string]Entry {
	entries := make(map[string]Entry, len(values))
	for key, value := range values {
		entries[key] = Entry{
			Value: value,
			Kind:  kind,
		}
	}
	return entries
}

func cloneEntries(values map[string]Entry) map[string]Entry {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]Entry, len(values))
	for key, value := range values {
		value.Kind = normalizeKind(value.Kind)
		cloned[key] = value
	}
	return cloned
}

func cloneResolvedValues(values map[string]ResolvedValue) map[string]ResolvedValue {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]ResolvedValue, len(values))
	for key, value := range values {
		cloned[key] = cloneResolvedValue(value)
	}
	return cloned
}

func cloneResolvedValue(value ResolvedValue) ResolvedValue {
	value.Kind = normalizeKind(value.Kind)
	value.Overridden = cloneValueSources(value.Overridden)
	return value
}

func cloneValueSources(values []ValueSource) []ValueSource {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]ValueSource, len(values))
	copy(cloned, values)
	for i := range cloned {
		cloned[i].Kind = normalizeKind(cloned[i].Kind)
	}
	return cloned
}

func sortedEntryKeys(values map[string]Entry) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeKind(kind ConfigKind) ConfigKind {
	if kind == SecretConfig {
		return SecretConfig
	}
	return BehaviorConfig
}
