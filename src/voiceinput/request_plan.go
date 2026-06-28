package voiceinput

import (
	"net/url"
	"strconv"
	"strings"
)

const (
	VoiceStreamPath       = "/api/ws/speech_to_text/voice_stream"
	DefaultEncoding       = "linear16"
	DefaultSampleRate     = 16000
	DefaultChannels       = 1
	DefaultEndpointingMS  = 300
	DefaultSTTLanguage    = "en"
	DefaultAuthIsRequired = true
)

type STTRequestOptions struct {
	Language string
	Keyterms []string
}

type STTRequestPlan struct {
	Path          string
	Encoding      string
	SampleRate    int
	Channels      int
	EndpointingMS int
	Language      string
	Keyterms      []string
	AuthRequired  bool
}

func NewSTTRequestPlan(options STTRequestOptions) STTRequestPlan {
	language := strings.TrimSpace(options.Language)
	if language == "" {
		language = DefaultSTTLanguage
	}

	return STTRequestPlan{
		Path:          VoiceStreamPath,
		Encoding:      DefaultEncoding,
		SampleRate:    DefaultSampleRate,
		Channels:      DefaultChannels,
		EndpointingMS: DefaultEndpointingMS,
		Language:      language,
		Keyterms:      cleanKeyterms(options.Keyterms),
		AuthRequired:  DefaultAuthIsRequired,
	}
}

func (p STTRequestPlan) Query() url.Values {
	values := url.Values{}
	values.Set("encoding", firstNonEmpty(p.Encoding, DefaultEncoding))
	values.Set("sample_rate", strconv.Itoa(firstNonZero(p.SampleRate, DefaultSampleRate)))
	values.Set("channels", strconv.Itoa(firstNonZero(p.Channels, DefaultChannels)))
	values.Set("endpointing_ms", strconv.Itoa(firstNonZero(p.EndpointingMS, DefaultEndpointingMS)))
	values.Set("language", firstNonEmpty(p.Language, DefaultSTTLanguage))
	for _, term := range cleanKeyterms(p.Keyterms) {
		values.Add("keyterms", term)
	}
	return values
}

func (p STTRequestPlan) RequestURI() string {
	return firstNonEmpty(p.Path, VoiceStreamPath) + "?" + p.Query().Encode()
}

func cleanKeyterms(terms []string) []string {
	if len(terms) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term != "" {
			cleaned = append(cleaned, term)
		}
	}
	return cleaned
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstNonZero(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
