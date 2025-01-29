package opts

type CLIArgs struct {
	Truncate         bool
	Preserve         bool
	DeferConstraints bool
	DisableTriggers  bool
	NoSafety         bool
	SkipConfirmation bool
	SyncConfigPath   string
	Groups           []string
	Tables           []string
	Excluded         []string
}
