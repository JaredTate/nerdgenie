// The three shapes of a schedule, and working the next moment out in the job's
// own time zone rather than in the machine's, are ZeroClaw's design, at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/cron/types.rs and
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/cron/schedule.rs. The backoff after
// a failed run climbing through a fixed table is OpenClaw's, at
// ~/Code/openclaw/src/cron/service/jobs-scheduling.ts. The Go here is written
// fresh.

package job

import (
	"errors"
	"fmt"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/robfig/cron/v3"
)

// shortestInterval is the closest together two ticks of a schedule may be. A
// task takes minutes, so a schedule asking for one every second is asking for
// work that cannot be done and is refused when the job is made.
const shortestInterval = time.Minute

// backoffSteps is how far a failed tick pushes the next one back: half a minute
// at first, and an hour once a job has been failing for a while. A job that is
// merely waiting for a website to come back does not hammer it, and a job whose
// trouble has passed loses at most an hour.
var backoffSteps = []time.Duration{
	30 * time.Second,
	60 * time.Second,
	5 * time.Minute,
	15 * time.Minute,
	60 * time.Minute,
}

// checkTheSchedule says whether a schedule can be read at all, and says plainly
// what is wrong when it cannot. A job with no schedule is fine and returns no
// error.
func checkTheSchedule(schedule *contract.Schedule) error {
	if schedule == nil {
		return nil
	}
	if _, err := placeOf(*schedule); err != nil {
		return err
	}
	switch schedule.Kind {
	case contract.ScheduleAt:
		if schedule.At.IsZero() {
			return errors.New("a schedule of the at kind runs once at one moment, so pass the moment it runs")
		}
	case contract.ScheduleEvery:
		if schedule.Every < shortestInterval {
			return fmt.Errorf("a schedule of every %s is closer together than the %s a task takes, so ask for it less often",
				schedule.Every, shortestInterval)
		}
	case contract.ScheduleCron:
		if _, err := cron.ParseStandard(schedule.Cron); err != nil {
			return fmt.Errorf("the cron expression %q cannot be read, so write five fields such as %q: %w",
				schedule.Cron, "0 7 * * 1-5", err)
		}
	default:
		return fmt.Errorf("the schedule kind %q is none of the three a job has, so use %q, %q, or %q",
			schedule.Kind, contract.ScheduleAt, contract.ScheduleEvery, contract.ScheduleCron)
	}
	return nil
}

// nextRun is the first moment after the one given at which a schedule fires. It
// returns the zero time when the schedule has no moment left, which is what a
// one-off schedule that has already run has.
func nextRun(schedule contract.Schedule, after time.Time) (time.Time, error) {
	place, err := placeOf(schedule)
	if err != nil {
		return time.Time{}, err
	}
	switch schedule.Kind {
	case contract.ScheduleAt:
		if !schedule.At.After(after) {
			return time.Time{}, nil
		}
		return schedule.At, nil
	case contract.ScheduleEvery:
		if schedule.Every < shortestInterval {
			return time.Time{}, fmt.Errorf("a schedule of every %s is closer together than the %s a task takes, so ask for it less often",
				schedule.Every, shortestInterval)
		}
		return after.Add(schedule.Every), nil
	case contract.ScheduleCron:
		expression, err := cron.ParseStandard(schedule.Cron)
		if err != nil {
			return time.Time{}, fmt.Errorf("the cron expression %q cannot be read, so write five fields such as %q: %w",
				schedule.Cron, "0 7 * * 1-5", err)
		}
		return expression.Next(after.In(place)), nil
	default:
		return time.Time{}, fmt.Errorf("the schedule kind %q is none of the three a job has, so use %q, %q, or %q",
			schedule.Kind, contract.ScheduleAt, contract.ScheduleEvery, contract.ScheduleCron)
	}
}

// placeOf is the time zone a schedule is read in. A schedule that names none is
// read in the machine's own, which is the one the user set on it.
func placeOf(schedule contract.Schedule) (*time.Location, error) {
	if schedule.Timezone == "" {
		return time.Local, nil
	}
	place, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return nil, fmt.Errorf("the time zone %q is not one this machine knows, so name one such as %q: %w",
			schedule.Timezone, "America/New_York", err)
	}
	return place, nil
}

// nextBackoff is how far the next tick is pushed back after another failure in a
// row. It climbs through the table and stays at the top of it.
func nextBackoff(failuresInARow int) time.Duration {
	if failuresInARow < 1 {
		return 0
	}
	step := failuresInARow - 1
	if step >= len(backoffSteps) {
		step = len(backoffSteps) - 1
	}
	return backoffSteps[step]
}

// afterAFailedTick is when a scheduled job runs next after a tick failed: the
// schedule's own next moment, or the end of the backoff when that is later. The
// backoff never cancels a run, it only delays one.
func afterAFailedTick(schedule contract.Schedule, now time.Time, failuresInARow int) (time.Time, time.Duration, error) {
	waiting := nextBackoff(failuresInARow)
	fromSchedule, err := nextRun(schedule, now)
	if err != nil {
		return time.Time{}, 0, err
	}
	held := now.Add(waiting)
	if fromSchedule.IsZero() {
		return time.Time{}, waiting, nil
	}
	if fromSchedule.After(held) {
		return fromSchedule, waiting, nil
	}
	return held, waiting, nil
}
