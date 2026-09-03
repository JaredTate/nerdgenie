// The claim on a running task is ZeroClaw's design, at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/cron/store.rs, where a job is
// taken with one conditional update rather than a read followed by a write, and
// the number of rows the statement changed is the answer to "did I win". The Go
// here is written fresh, and the condition is the deadline rather than a null
// column, so a claim left behind by a process that died is taken by the next one
// to ask once the task's budget has run out.

package job

import (
	"context"
	"fmt"
	"time"
)

// claimsTable is the one table this package owns in the shared database file.
// One row is one task that some process is running now.
const claimsTable = `CREATE TABLE IF NOT EXISTS job_claims (
	job_id   TEXT NOT NULL,
	task_id  TEXT NOT NULL,
	owner    TEXT NOT NULL,
	taken_at TEXT NOT NULL,
	deadline TEXT NOT NULL,
	PRIMARY KEY (job_id, task_id)
)`

// createTables makes this package's own table in the database file.
func (jobs *Jobs) createTables(ctx context.Context) error {
	if _, err := jobs.database.ExecContext(ctx, claimsTable); err != nil {
		return fmt.Errorf("cannot make the table of job claims in %s: %w", jobs.home.DatabaseFile(), err)
	}
	return nil
}

// claim takes one task for this process and says whether it won. The whole of
// the decision is one statement, so two processes asking at the same moment
// cannot both be told yes: the second one changes no rows, because the row is
// already there and its deadline has not passed.
func (jobs *Jobs) claim(ctx context.Context, jobID string, taskID string, now time.Time) (bool, error) {
	result, err := jobs.database.ExecContext(ctx,
		`INSERT INTO job_claims (job_id, task_id, owner, taken_at, deadline)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (job_id, task_id) DO UPDATE SET
		   owner = excluded.owner, taken_at = excluded.taken_at, deadline = excluded.deadline
		 WHERE job_claims.deadline <= ?`,
		jobID, taskID, jobs.owner, stamp(now), stamp(now.Add(TaskBudget)), stamp(now))
	if err != nil {
		return false, fmt.Errorf("cannot claim task %s of job %s: %w", taskID, jobID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot tell whether task %s of job %s was claimed: %w", taskID, jobID, err)
	}
	return changed == 1, nil
}

// release gives up the claim on one task, which is what finishing it does.
func (jobs *Jobs) release(ctx context.Context, jobID string, taskID string) error {
	_, err := jobs.database.ExecContext(ctx,
		`DELETE FROM job_claims WHERE job_id = ? AND task_id = ?`, jobID, taskID)
	if err != nil {
		return fmt.Errorf("cannot release the claim on task %s of job %s: %w", taskID, jobID, err)
	}
	return nil
}

// claimedTasks returns the tasks of one job that some process is running now and
// whose budget has not run out, which are the ones nobody else may start.
func (jobs *Jobs) claimedTasks(ctx context.Context, jobID string, now time.Time) (map[string]bool, error) {
	rows, err := jobs.database.QueryContext(ctx,
		`SELECT task_id FROM job_claims WHERE job_id = ? AND deadline > ?`, jobID, stamp(now))
	if err != nil {
		return nil, fmt.Errorf("cannot read the claims on the tasks of job %s: %w", jobID, err)
	}
	defer rows.Close()

	running := map[string]bool{}
	for rows.Next() {
		taskID := ""
		if err := rows.Scan(&taskID); err != nil {
			return nil, fmt.Errorf("cannot read a claim on a task of job %s: %w", jobID, err)
		}
		running[taskID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the claims on the tasks of job %s to the end: %w", jobID, err)
	}
	return running, nil
}

// expiredClaims returns every claim whose task has been running longer than its
// budget, so that the task can be counted as a failure and offered again.
func (jobs *Jobs) expiredClaims(ctx context.Context, now time.Time) ([][2]string, error) {
	rows, err := jobs.database.QueryContext(ctx,
		`SELECT job_id, task_id FROM job_claims WHERE deadline <= ? ORDER BY job_id, task_id`, stamp(now))
	if err != nil {
		return nil, fmt.Errorf("cannot read the claims whose budget has run out: %w", err)
	}
	defer rows.Close()

	overdue := [][2]string{}
	for rows.Next() {
		jobID, taskID := "", ""
		if err := rows.Scan(&jobID, &taskID); err != nil {
			return nil, fmt.Errorf("cannot read a claim whose budget has run out: %w", err)
		}
		overdue = append(overdue, [2]string{jobID, taskID})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the claims whose budget has run out to the end: %w", err)
	}
	return overdue, nil
}

// stampLayout is how a moment is written in the claims table. Every field is
// padded to a fixed width, in one time zone, so that comparing two stamps as
// text gives the same answer as comparing the two moments. A layout that drops
// trailing zeroes, as the usual one does, would not.
const stampLayout = "2006-01-02T15:04:05.000000000Z"

// stamp writes a moment the way every row of the claims table holds it.
func stamp(moment time.Time) string {
	return moment.UTC().Format(stampLayout)
}
