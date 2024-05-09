package sync

import (
	"errors"
	"fmt"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/db"
	"github.com/jwbonnell/pggosync/opts"
)

type TaskResolver struct {
	Config *config.Config
}

func NewTaskResolver(cfg *config.Config) *TaskResolver {
	return &TaskResolver{Config: cfg}
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

	/* TODO determine if individual table syncs will be supported and
	 *
	for i := range tableArgs {
		newTasks, err := tr.tableToTasks(tableArgs[i])
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, newTasks...)
	}*/

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
		schema, table, _, err := opts.ParseTableArg(tkey)
		if err != nil {
			return nil, err
		}

		filter = opts.ApplyParamToFilter(params, filter)

		tasks = append(tasks, Task{
			Table:    db.Table{Schema: schema, Name: table},
			Filter:   filter,
			Truncate: true,
		})
	}
	return tasks, nil
}

/*func (tr *TaskResolver) tableToTasks(tableArgs string) ([]Task, error) {
	schema, table, params, err := opts.ParseTableArg(tableArgs)
	if err != nil {
		return nil, err
	}

	filter = opts.ApplyParamToFilter(params, filter)

	task := Task{
		Table:    db.Table{Schema: schema, Name: table},
		Filter:   filter,
		Truncate: true,
	}
}*/
