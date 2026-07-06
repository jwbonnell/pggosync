package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync/data"
	"github.com/jwbonnell/pggosync/sync/table"
)

type TaskResolver struct {
	source           *datasource.ReaderDataSource
	destination      *datasource.ReadWriteDatasource
	groups           map[string]config.Group
	truncate         bool
	preserve         bool
	deferConstraints bool
	disableTriggers  bool
	excluded         []db.Table
}

// NewTaskResolver creates a TaskResolver with all sync options baked in for use across multiple Resolve calls.
func NewTaskResolver(source *datasource.ReaderDataSource, destination *datasource.ReadWriteDatasource, groups map[string]config.Group, truncate bool, preserve bool, deferConstraints bool, disableTriggers bool, excluded []db.Table) *TaskResolver {
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

// Resolve expands group and table args into a []Task with columns, PKs, and sequences loaded.
// Falls back to all shared tables when neither groups nor tables are specified.
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

	if err := validateScrubRules(tasks); err != nil {
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

	if err := validateScrubColumns(tasks); err != nil {
		return nil, err
	}

	var missingPK []string
	for i, task := range tasks {
		if (!task.Truncate || task.Preserve) && len(task.DestPK) == 0 {
			missingPK = append(missingPK, task.Table.FullName())
		}

		if task.Truncate && !task.Preserve {
			count, err := tr.destination.GetRowCount(ctx, task.Table.FullName())
			if err != nil {
				return nil, err
			}
			tasks[i].DestRowCount = count
		}
	}
	if len(missingPK) > 0 {
		return nil, fmt.Errorf("tables require a primary key for upsert sync (use --truncate or add a PK): %s", strings.Join(missingPK, ", "))
	}

	return tasks, nil
}

// groupToTasks converts a single group arg (with optional params) into tasks, substituting params into filters.
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
	for _, entry := range group.Tables {
		schema, tab, err := opts.ParseFullTableName(entry.Table)
		if err != nil {
			return nil, err
		}

		filter := opts.ApplyParamToFilter(params, entry.Filter)

		truncate := tr.truncate
		if entry.Truncate != nil {
			truncate = *entry.Truncate
		}
		preserve := tr.preserve
		if entry.Preserve != nil {
			preserve = *entry.Preserve
		}

		tasks = append(tasks, Task{
			Table:            db.Table{Schema: schema, Name: tab},
			Filter:           filter,
			Preserve:         preserve,
			Truncate:         truncate,
			DeferConstraints: tr.deferConstraints,
			ScrubRules:       entry.Scrub,
		})
	}
	return tasks, nil
}

// tableToTasks converts an explicit --table arg into a Task; errors if the table appears on the exclusion list.
func (tr *TaskResolver) tableToTasks(tableArgs string, excluded []db.Table) (Task, error) {
	parsed, err := opts.ParseTableArgWithScrub(tableArgs)
	if err != nil {
		return Task{}, err
	}

	t := db.Table{Schema: parsed.Schema, Name: parsed.Table}
	if len(excluded) > 0 && len(table.FilterTables([]db.Table{t}, excluded)) == 0 {
		return Task{}, fmt.Errorf("supplied table %s is in the excluded list", tableArgs)
	}

	return Task{
		Table:      t,
		Filter:     parsed.Filter,
		Truncate:   true,
		ScrubRules: parsed.ScrubRules,
	}, nil
}

// loadColumns fetches columns from both datasources and returns two maps keyed by "schema.table".
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

// loadPrimaryKeys fetches destination PKs and returns a map keyed by "schema.table".
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

// loadSequences fetches sequences from both datasources and returns two maps keyed by "schema.table".
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

// confirmTablesExist returns an error naming the first table absent from the given datasource.
func confirmTablesExist(ds datasource.ReadDataSource, tasks []Task) error {
	for i := range tasks {
		if !ds.TableExists(tasks[i].Table) {
			return fmt.Errorf("table %s does not exist in datasource %s", tasks[i].Table.FullName(), ds.GetName())
		}
	}
	return nil
}

// validateScrubRules checks that every scrub rule on every task uses a supported rule ID.
func validateScrubRules(tasks []Task) error {
	for _, task := range tasks {
		for _, rule := range task.ScrubRules {
			if !data.IsValidRule(rule.Rule) {
				return fmt.Errorf("table %s: unsupported scrub rule %q (valid: %s)", task.FullName(), rule.Rule, strings.Join(data.SupportedRules, ", "))
			}
			if rule.Column == "" {
				return fmt.Errorf("table %s: scrub rule missing column name", task.FullName())
			}
		}
	}
	return nil
}

// validateScrubColumns checks, after column and PK metadata is loaded, that every scrub rule
// targets an existing shared column and never a primary key column. A misspelled column would
// otherwise silently sync unscrubbed data; a scrubbed PK would break upsert conflict detection.
func validateScrubColumns(tasks []Task) error {
	for _, task := range tasks {
		if len(task.ScrubRules) == 0 {
			continue
		}

		shared := make(map[string]bool)
		for _, col := range task.GetSharedColumnNames() {
			shared[strings.Trim(col, `"`)] = true
		}
		pks := make(map[string]bool)
		for _, pk := range task.GetDestPKs() {
			pks[strings.Trim(pk, `"`)] = true
		}

		for _, rule := range task.ScrubRules {
			if !shared[rule.Column] {
				return fmt.Errorf("table %s: scrub column %q does not exist in both source and destination", task.FullName(), rule.Column)
			}
			if pks[rule.Column] {
				return fmt.Errorf("table %s: scrub column %q is a primary key column and cannot be scrubbed", task.FullName(), rule.Column)
			}
		}
	}
	return nil
}
