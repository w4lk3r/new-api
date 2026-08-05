package constant

type TaskPlatform string

const (
	TaskPlatformSuno       TaskPlatform = "suno"
	TaskPlatformMidjourney              = "mj"
	// Task platforms are protocol identifiers, not channel-type numbers. This
	// keeps existing tasks routable when a private deployment later adds more
	// native-video providers or reconciles channel type numbers with upstream.
	TaskPlatformLingjingVideo       TaskPlatform = NativeVideoAdapterLingjing
	LegacyTaskPlatformLingjingVideo TaskPlatform = "59"
)

const (
	SunoActionMusic  = "MUSIC"
	SunoActionLyrics = "LYRICS"

	TaskActionGenerate          = "generate"
	TaskActionTextGenerate      = "textGenerate"
	TaskActionFirstTailGenerate = "firstTailGenerate"
	TaskActionReferenceGenerate = "referenceGenerate"
	TaskActionRemix             = "remixGenerate"
)

var SunoModel2Action = map[string]string{
	"suno_music":  SunoActionMusic,
	"suno_lyrics": SunoActionLyrics,
}
