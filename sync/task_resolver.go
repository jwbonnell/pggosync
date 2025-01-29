package sync

import (
	"context"
	"errors"
	"fmt"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync/table"
)

type TaskResolver struct {
	source           *datasource.ReaderDataSource
	destination      *datasource.ReadWriteDatasource
	groups           map[string]map[string]string
	truncate         bool
	preserve         bool
	deferConstraints bool
	disableTriggers  bool
	excluded         []db.Table
}

func NewTaskResolver(source *datasource.ReaderDataSource, destination *datasource.ReadWriteDatasource, groups map[string]map[string]string, truncate bool, preserve bool, deferConstraints bool, disableTriggers bool, excluded []db.Table) *TaskResolver {
	return &TaskResolver{
		source:           source,
		destination:      destination,
		truncate:         truncate,
		groups:           groups,
		preserve:         preserve,
		deferConstraints: deferConstraints,
		disableTriggers:  disableTriggers,
		excluded:         excluded}
}

func (tr *TaskResolver) Resolve(ctx context.Context, groupArgs []string, tableArgs []string) ([]Task, error) {
	var tasks []Task

	for i := range groupArgs {
		newTasks, err := tr.groupToTasks(groupArgs[i])
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, newTasks...)
	}

	for i := range tableArgs {
		newTask, err := tr.tableToTasks(tableArgs[i], tr.excluded)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, newTask)
	}

	if len(groupArgs) == 0 && len(tableArgs) == 0 {
		//No groups or tables passed in, assume all tables are desired.
		sharedTables := table.GetSharedTables(tr.source, tr.destination, tr.excluded)
		for _, t := range sharedTables {
			tasks = append(tasks, Task{
				Table:    t,
				Filter:   "",
				Truncate: tr.truncate,
			})
		}
	}

	if err := confirmTablesExist(tr.source, tasks); err != nil {
		return nil, err
	}

	if err := confirmTablesExist(tr.destination, tasks); err != nil {
		return nil, err
	}

	sourceColumns, destinationColumns, err := tr.loadColumns(ctx)
	if err != nil {
		return nil, err
	}

	destPKs, err := tr.loadPrimaryKeys(ctx)
	if err != nil {
		return nil, err
	}

	destSeq, sourceSeq, err := tr.loadSequences(ctx)
	if err != nil {
		return nil, err
	}

	for i := range tasks {
		sc, ok := sourceColumns[tasks[i].FullName()]
		if ok {
			tasks[i].SourceColumns = sc
		}

		dc, ok := destinationColumns[tasks[i].FullName()]
		if ok {
			tasks[i].DestColumns = dc
		}

		dpk, ok := destPKs[tasks[i].FullName()]
		if ok {
			tasks[i].DestPK = dpk
		}

		destSeq, ok := destSeq[tasks[i].FullName()]
		if ok {
			tasks[i].DestSequences = destSeq
		}

		sourceSeq, ok := sourceSeq[tasks[i].FullName()]
		if ok {
			tasks[i].SourceSequences = sourceSeq
		}
	}

	return tasks, nil
}

func (tr *TaskResolver) groupToTasks(groupArg string) ([]Task, error) {
	groupID, params, err := opts.ParseGroupArg(groupArg)
	if err != nil {
		return nil, fmt.Errorf("TaskResolver.groupToTasks %w", err)
	}

	group, ok := tr.groups[groupID]
	if !ok {
		return nil, errors.New("No such group " + groupID)
	}

	var tasks []Task
	for tkey, filter := range group {
		schema, tab, err := opts.ParseFullTableName(tkey)
		if err != nil {
			return nil, err
		}

		filter = opts.ApplyParamToFilter(params, filter)

		tasks = append(tasks, Task{
			Table:            db.Table{Schema: schema, Name: tab},
			Filter:           filter,
			Preserve:         tr.preserve,
			Truncate:         tr.truncate,
			DeferConstraints: tr.deferConstraints,
		})
	}
	return tasks, nil //--table users:"WHERE something" or
}

func (tr *TaskResolver) tableToTasks(tableArgs string, excluded []db.Table) (Task, error) {
	schema, tableName, filter, err := opts.ParseTableArg(tableArgs)
	if err != nil {
		return Task{}, err
	}

	t := db.Table{Schema: schema, Name: tableName}
	if len(excluded) > 0 && len(table.FilterTables([]db.Table{t}, excluded)) == 0 {
		return Task{}, fmt.Errorf("supplied table %s is in the excluded list", tableArgs)
	}

	return Task{
		Table:    t,
		Filter:   filter,
		Truncate: true,
	}, nil
}

func (tr *TaskResolver) loadColumns(ctx context.Context) (map[string][]db.Column, map[string][]db.Column, error) {
	sourceColumns, err := tr.source.GetColumns(ctx)
	if err != nil {
		return nil, nil, err
	}

	sourceMap := make(map[string][]db.Column)
	for i := range sourceColumns {
		fullName := sourceColumns[i].Schema + "." + sourceColumns[i].Table
		sourceMap[fullName] = append(sourceMap[fullName], sourceColumns[i])
	}

	destColumns, err := tr.destination.GetColumns(ctx)
	if err != nil {
		return nil, nil, err
	}

	destMap := make(map[string][]db.Column)
	for i := range destColumns {
		fullName := destColumns[i].Schema + "." + destColumns[i].Table
		destMap[fullName] = append(destMap[fullName], destColumns[i])
	}

	return sourceMap, destMap, nil
}

func (tr *TaskResolver) loadPrimaryKeys(ctx context.Context) (map[string][]db.PrimaryKey, error) {
	pks, err := tr.destination.GetPrimaryKeys(ctx)
	if err != nil {
		return nil, err
	}

	pkMap := make(map[string][]db.PrimaryKey)
	for i := range pks {
		fullName := pks[i].Schema + "." + pks[i].Table
		pkMap[fullName] = append(pkMap[fullName], pks[i])
	}

	return pkMap, nil
}

func (tr *TaskResolver) loadSequences(ctx context.Context) (map[string][]db.Sequence, map[string][]db.Sequence, error) {
	sourceSeq, err := tr.source.GetSequences(ctx)
	if err != nil {
		return nil, nil, err
	}

	sourceMap := make(map[string][]db.Sequence)
	for i := range sourceSeq {
		fullName := sourceSeq[i].Schema + "." + sourceSeq[i].Table
		sourceMap[fullName] = append(sourceMap[fullName], sourceSeq[i])
	}

	destSeq, err := tr.destination.GetSequences(ctx)
	if err != nil {
		return nil, nil, err
	}

	destMap := make(map[string][]db.Sequence)
	for i := range destSeq {
		fullName := destSeq[i].Schema + "." + destSeq[i].Table
		destMap[fullName] = append(destMap[fullName], destSeq[i])
	}

	return sourceMap, destMap, nil
}

func confirmTablesExist(ds datasource.ReadDataSource, tasks []Task) error {
	for i := range tasks {
		if ds.TableExists(tasks[i].Table) {
			return nil
		}
	}
	return fmt.Errorf("table %s does not exist in datasource %s", tasks[0].Table, ds.GetName())
}
