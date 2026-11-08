package capture

// Audio from CaptureAudio is 16-bit little-endian PCM in this layout.
const (
	AudioRate     = 44100
	AudioChannels = 2
	AudioBits     = 16
)

// AudioFrameBytes is the size of one sample for all channels.
const AudioFrameBytes = AudioChannels * AudioBits / 8
