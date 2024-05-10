package sync

import (
	"github.com/jwbonnell/pggosync/db"
)

type Task struct {
	db.Table
	Filter          string
	Preserve        bool
	Truncate        bool
	DeferContraints bool
}
