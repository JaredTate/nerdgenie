package browser

import (
	"fmt"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// dailyBudget counts what the agent has done on each site today. Design section
// 9 says every website gets a daily budget of actions, and this is where that
// number is kept: one count per hostname, thrown away when the day turns over on
// the clock this package reads.
type dailyBudget struct {
	guard sync.Mutex
	clock contract.Clock
	limit int
	used  map[string]siteCount
}

// siteCount is what one hostname has spent, and the day it spent it on.
type siteCount struct {
	// day is the calendar day the count belongs to, as the clock reads it.
	day string
	// actions is how many actions have been spent on that day.
	actions int
}

// newDailyBudget returns a budget of that many actions per hostname per day.
func newDailyBudget(clock contract.Clock, limit int) *dailyBudget {
	return &dailyBudget{clock: clock, limit: limit, used: map[string]siteCount{}}
}

// charge spends actions from one hostname's budget for today, and refuses the
// call when there are not that many left, saying the number and when the budget
// comes back. A call with no hostname behind it, which is a page that has not
// been opened yet, is not charged to anybody.
func (budget *dailyBudget) charge(hostname string, actions int) error {
	if hostname == "" || actions <= 0 {
		return nil
	}
	budget.guard.Lock()
	defer budget.guard.Unlock()

	now := budget.clock.Now()
	today := now.Format(time.DateOnly)
	spent := budget.used[hostname]
	if spent.day != today {
		spent = siteCount{day: today}
	}
	if spent.actions+actions > budget.limit {
		return fmt.Errorf("the browser has used %d of its %d actions on %s today, so it will not act there again until the budget resets at %s: work on something else, or ask the user to raise the daily budget",
			spent.actions, budget.limit, hostname, resetsAt(now).Format(time.RFC1123))
	}
	spent.actions += actions
	budget.used[hostname] = spent
	return nil
}

// spent is how many actions one hostname has used today, which a test reads.
func (budget *dailyBudget) spent(hostname string) int {
	budget.guard.Lock()
	defer budget.guard.Unlock()
	held := budget.used[hostname]
	if held.day != budget.clock.Now().Format(time.DateOnly) {
		return 0
	}
	return held.actions
}

// resetsAt is the midnight after the moment given, in that moment's own time
// zone, which is when every site's count starts again.
func resetsAt(now time.Time) time.Time {
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
}
