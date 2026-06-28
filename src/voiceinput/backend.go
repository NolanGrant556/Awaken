package voiceinput

import "strings"

type RecordingBackend string

const (
	RecordingBackendNative      RecordingBackend = "native"
	RecordingBackendArecord     RecordingBackend = "arecord"
	RecordingBackendSox         RecordingBackend = "sox"
	RecordingBackendUnsupported RecordingBackend = "unsupported"
	RecordingBackendHeadless    RecordingBackend = "headless"
	RecordingBackendRemote      RecordingBackend = "remote"
)

const (
	PlatformDarwin  = "darwin"
	PlatformLinux   = "linux"
	PlatformWindows = "windows"
)

type RecordingEnvironment struct {
	Platform        string
	Remote          bool
	Headless        bool
	NativeAvailable bool
	HasArecord      bool
	ArecordUsable   bool
	HasSox          bool
}

type RecordingAvailability struct {
	Available bool
	Backend   RecordingBackend
	Reason    string
}

func DecideRecordingBackend(env RecordingEnvironment) RecordingAvailability {
	if env.Remote {
		return RecordingAvailability{
			Available: false,
			Backend:   RecordingBackendRemote,
			Reason:    "voice input requires a local microphone",
		}
	}

	if env.Headless {
		return RecordingAvailability{
			Available: false,
			Backend:   RecordingBackendHeadless,
			Reason:    "voice input is unavailable in a headless environment",
		}
	}

	if env.NativeAvailable {
		return RecordingAvailability{
			Available: true,
			Backend:   RecordingBackendNative,
		}
	}

	platform := strings.ToLower(strings.TrimSpace(env.Platform))
	if platform == PlatformWindows {
		return RecordingAvailability{
			Available: false,
			Backend:   RecordingBackendUnsupported,
			Reason:    "windows voice input requires the native recorder",
		}
	}

	if platform == PlatformLinux && env.HasArecord && env.ArecordUsable {
		return RecordingAvailability{
			Available: true,
			Backend:   RecordingBackendArecord,
		}
	}

	if env.HasSox {
		return RecordingAvailability{
			Available: true,
			Backend:   RecordingBackendSox,
		}
	}

	return RecordingAvailability{
		Available: false,
		Backend:   RecordingBackendUnsupported,
		Reason:    "no supported voice recording backend is available",
	}
}
