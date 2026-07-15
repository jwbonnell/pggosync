package opts

type CLIArgs struct {
	Truncate         bool
	Cascade          bool
	Preserve         bool
	DeferConstraints bool
	DisableTriggers  bool
	NoSafety         bool
	SkipConfirmation bool
	Quiet            bool
	DryRun           bool
	Verify           bool
	Concurrency      int
	BufferSize       int // per-table prefetch buffer cap in MiB
	SyncConfigPath   string
	Groups           []string
	Tables           []string
	Excluded         []string
}
