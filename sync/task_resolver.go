package sync

import (
	"errors"
	"fmt"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/db"
	"github.com/jwbonnell/pggosync/opts"
)

type TaskResolver struct {
	Config           *config.Config
	truncate         bool
	preserve         bool
	deferConstraints bool
}

func NewTaskResolver(cfg *config.Config, truncate bool, preserve bool, deferConstraints bool) *TaskResolver {
	return &TaskResolver{Config: cfg, truncate: truncate, preserve: preserve, deferConstraints: deferConstraints}
}

func (tr *TaskResolver) Resolve(groupArgs []string, tableArgs []string) ([]Task, error) {
	var tasks []Task

	for i := range groupArgs {
		newTasks, err := tr.groupToTasks(groupArgs[i])
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, newTasks...)
	}

	for i := range tableArgs {
		newTask, err := tr.tableToTasks(tableArgs[i])
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, newTask)
	}

	if len(groupArgs) == 0 && len(tableArgs) == 0 {

	}

	return tasks, nil
}

func (tr *TaskResolver) groupToTasks(groupArg string) ([]Task, error) {

	groupID, params, err := opts.ParseGroupArg(groupArg)
	if err != nil {
		return nil, fmt.Errorf("TaskResolver.groupToTasks %w", err)
	}

	group, ok := tr.Config.Groups[groupID]
	if !ok {
		return nil, errors.New("No such group " + groupID)
	}

	var tasks []Task
	for tkey, filter := range group {
		schema, table, err := opts.ParseFullTableName(tkey)
		if err != nil {
			return nil, err
		}

		filter = opts.ApplyParamToFilter(params, filter)

		tasks = append(tasks, Task{
			Table:            db.Table{Schema: schema, Name: table},
			Filter:           filter,
			Preserve:         tr.preserve,
			Truncate:         tr.truncate,
			DeferConstraints: tr.deferConstraints,
		})
	}
	return tasks, nil //--table users:"WHERE something" or
}

func (tr *TaskResolver) tableToTasks(tableArgs string) (Task, error) {
	schema, table, filter, err := opts.ParseTableArg(tableArgs)
	if err != nil {
		return Task{}, err
	}

	return Task{
		Table:    db.Table{Schema: schema, Name: table},
		Filter:   filter,
		Truncate: true,
	}, nil
}
