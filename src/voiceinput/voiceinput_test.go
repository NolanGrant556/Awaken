package voiceinput

import (
	"net/url"
	"testing"
)

func TestDecideRecordingBackendFallback(t *testing.T) {
	tests := []struct {
		name string
		env  RecordingEnvironment
		want RecordingBackend
	}{
		{
			name: "native wins when available",
			env: RecordingEnvironment{
				Platform:        PlatformLinux,
				NativeAvailable: true,
				HasArecord:      true,
				ArecordUsable:   true,
				HasSox:          true,
			},
			want: RecordingBackendNative,
		},
		{
			name: "linux falls back to usable arecord",
			env: RecordingEnvironment{
				Platform:      PlatformLinux,
				HasArecord:    true,
				ArecordUsable: true,
				HasSox:        true,
			},
			want: RecordingBackendArecord,
		},
		{
			name: "linux skips broken arecord and falls back to sox",
			env: RecordingEnvironment{
				Platform:      PlatformLinux,
				HasArecord:    true,
				ArecordUsable: false,
				HasSox:        true,
			},
			want: RecordingBackendSox,
		},
		{
			name: "mac can use sox when native is absent",
			env: RecordingEnvironment{
				Platform: PlatformDarwin,
				HasSox:   true,
			},
			want: RecordingBackendSox,
		},
		{
			name: "windows has no non-native fallback",
			env: RecordingEnvironment{
				Platform: PlatformWindows,
				HasSox:   true,
			},
			want: RecordingBackendUnsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideRecordingBackend(tt.env)
			if got.Backend != tt.want {
				t.Fatalf("expected backend %s, got %+v", tt.want, got)
			}
			if got.Available != (tt.want != RecordingBackendUnsupported) {
				t.Fatalf("unexpected availability for backend %s: %+v", tt.want, got)
			}
		})
	}
}

func TestDecideRecordingBackendRemoteHeadlessUnavailable(t *testing.T) {
	remote := DecideRecordingBackend(RecordingEnvironment{
		Platform:        PlatformDarwin,
		Remote:          true,
		NativeAvailable: true,
	})
	if remote.Available {
		t.Fatalf("remote environment should be unavailable: %+v", remote)
	}
	if remote.Backend != RecordingBackendRemote {
		t.Fatalf("expected remote backend, got %+v", remote)
	}

	headless := DecideRecordingBackend(RecordingEnvironment{
		Platform:        PlatformLinux,
		Headless:        true,
		NativeAvailable: true,
	})
	if headless.Available {
		t.Fatalf("headless environment should be unavailable: %+v", headless)
	}
	if headless.Backend != RecordingBackendHeadless {
		t.Fatalf("expected headless backend, got %+v", headless)
	}
}

func TestSTTRequestPlanKeytermsQueryParams(t *testing.T) {
	plan := NewSTTRequestPlan(STTRequestOptions{
		Language: "zh",
		Keyterms: []string{
			"Awaken",
			"voice input",
			"",
			"worktree",
		},
	})

	if !plan.AuthRequired {
		t.Fatalf("expected auth to be required")
	}

	uri, err := url.Parse(plan.RequestURI())
	if err != nil {
		t.Fatalf("parse request URI: %v", err)
	}
	if uri.Path != VoiceStreamPath {
		t.Fatalf("expected path %q, got %q", VoiceStreamPath, uri.Path)
	}

	values := uri.Query()
	assertQueryValue(t, values, "encoding", "linear16")
	assertQueryValue(t, values, "sample_rate", "16000")
	assertQueryValue(t, values, "channels", "1")
	assertQueryValue(t, values, "endpointing_ms", "300")
	assertQueryValue(t, values, "language", "zh")

	keyterms := values["keyterms"]
	want := []string{"Awaken", "voice input", "worktree"}
	if len(keyterms) != len(want) {
		t.Fatalf("expected keyterms %v, got %v", want, keyterms)
	}
	for i := range want {
		if keyterms[i] != want[i] {
			t.Fatalf("expected keyterms %v, got %v", want, keyterms)
		}
	}
}

func TestCloseStreamDropsAudioFrames(t *testing.T) {
	session := NewSession()

	keepAlive := session.KeepAlive()
	if keepAlive.Kind != OutboundEventKeepAlive || keepAlive.Dropped {
		t.Fatalf("expected keepalive to be accepted, got %+v", keepAlive)
	}

	audio := session.SendAudioFrame([]byte("abc"))
	if audio.Kind != OutboundEventBinaryAudioFrame || audio.Dropped {
		t.Fatalf("expected audio frame to be accepted, got %+v", audio)
	}
	if string(audio.Payload) != "abc" {
		t.Fatalf("expected copied audio payload, got %q", string(audio.Payload))
	}

	closeStream := session.CloseStream()
	if closeStream.Kind != OutboundEventCloseStream || closeStream.Dropped {
		t.Fatalf("expected CloseStream to be accepted, got %+v", closeStream)
	}
	if session.State() != StreamStateClosing {
		t.Fatalf("expected closing state, got %s", session.State())
	}

	dropped := session.SendAudioFrame([]byte("late"))
	if !dropped.Dropped || dropped.Reason != "close_stream_sent" {
		t.Fatalf("expected audio after CloseStream to be dropped, got %+v", dropped)
	}
}

func TestFinalizeIdempotent(t *testing.T) {
	session := NewSession()
	session.AddTranscriptText("hello from Awaken", false)
	session.CloseStream()

	first := session.Finalize(FinalizeSourceNoDataTimeout)
	if first.Source != FinalizeSourceNoDataTimeout {
		t.Fatalf("expected no_data_timeout, got %+v", first)
	}
	if first.Transcript != "hello from Awaken" {
		t.Fatalf("expected promoted transcript, got %q", first.Transcript)
	}
	if session.State() != StreamStateClosed {
		t.Fatalf("expected closed state, got %s", session.State())
	}

	second := session.Finalize(FinalizeSourceSafetyTimeout)
	if second.Source != FinalizeSourceAlreadyClosed {
		t.Fatalf("expected already_closed on second finalize, got %+v", second)
	}
	if second.Transcript != first.Transcript {
		t.Fatalf("expected transcript to remain %q, got %q", first.Transcript, second.Transcript)
	}

	dropped := session.SendAudioFrame([]byte("after-finalize"))
	if !dropped.Dropped || dropped.Reason != "stream_closed" {
		t.Fatalf("expected audio after finalize to be dropped, got %+v", dropped)
	}
}

func TestTranscriptTextEndpointAggregation(t *testing.T) {
	session := NewSession()

	update := session.AddTranscriptText("hel", false)
	if !update.Accepted || update.Final || update.Transcript != "hel" {
		t.Fatalf("unexpected partial update: %+v", update)
	}

	update = session.AddTranscriptText("hello world", true)
	if !update.Accepted || !update.Final || update.Transcript != "hello world" {
		t.Fatalf("unexpected final update: %+v", update)
	}

	update = session.AddTranscriptText("from Awaken", false)
	if !update.Accepted || update.Transcript != "hello world from Awaken" {
		t.Fatalf("unexpected second partial update: %+v", update)
	}

	endpoint := session.Endpoint()
	if !endpoint.Accepted || endpoint.Transcript != "hello world from Awaken" {
		t.Fatalf("unexpected endpoint update: %+v", endpoint)
	}
	if session.FinalTranscriptText() != "hello world from Awaken" {
		t.Fatalf("expected final transcript, got %q", session.FinalTranscriptText())
	}

	rejected := session.AddTranscriptText("ignored", false)
	if rejected.Accepted {
		t.Fatalf("expected transcript after endpoint to be rejected: %+v", rejected)
	}
	if session.TranscriptText() != "hello world from Awaken" {
		t.Fatalf("transcript changed after endpoint: %q", session.TranscriptText())
	}

	finalized := session.Finalize(FinalizeSourceEndpoint)
	if finalized.Source != FinalizeSourceEndpoint {
		t.Fatalf("expected endpoint finalize source, got %+v", finalized)
	}
}

func TestErrorClassification(t *testing.T) {
	auth := ClassifyError(ErrorEvent{
		Kind:       ErrorKindUpgradeRejected,
		HTTPStatus: 401,
	})
	if !auth.Fatal || auth.Ignored {
		t.Fatalf("expected HTTP 401 upgrade rejection to be fatal, got %+v", auth)
	}

	serviceUnavailable := ClassifyError(ErrorEvent{
		Kind:       ErrorKindUpgradeRejected,
		HTTPStatus: 503,
	})
	if serviceUnavailable.Fatal || serviceUnavailable.Ignored {
		t.Fatalf("expected HTTP 503 upgrade rejection to be nonfatal, got %+v", serviceUnavailable)
	}

	transcript := ClassifyError(ErrorEvent{
		Kind:    ErrorKindTranscript,
		Message: "decoder warning",
	})
	if transcript.Fatal || transcript.Ignored {
		t.Fatalf("expected transcript error to be nonfatal, got %+v", transcript)
	}

	normalClose := ClassifyError(ErrorEvent{
		Kind:      ErrorKindWebSocketClose,
		CloseCode: 1000,
	})
	if !normalClose.Ignored || normalClose.Fatal {
		t.Fatalf("expected normal close to be ignored, got %+v", normalClose)
	}
}

func assertQueryValue(t *testing.T, values url.Values, key, want string) {
	t.Helper()
	got := values.Get(key)
	if got != want {
		t.Fatalf("expected query %s=%q, got %q", key, want, got)
	}
}
