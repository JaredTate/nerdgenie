package record

import "github.com/JaredTate/nerdgenie/internal/contract"

// The two records in this file are the examples in section 4 of
// docs/NERDGENIE_PLAN.md, written out as the contract types. The text of each one is
// in testdata, taken from the design without a byte changed, and the printer and
// the parser are held to both.

// goldenTaskRecord is the task example from the design.
func goldenTaskRecord() contract.Record {
	return contract.Record{
		Header: contract.Header{
			Kind:        contract.RecordTask,
			ID:          "17",
			Status:      contract.StatusRunning,
			Origin:      "Signal",
			RoundsLeft:  86,
			MinutesLeft: 51,
			Cost:        contract.CostLine{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400},
		},
		Goal:    goldenTaskGoal(),
		Rules:   goldenTaskRules(),
		Work:    goldenTaskWork(),
		Lessons: goldenTaskLessons(),
	}
}

// goldenTaskGoal is the goal part of the task example.
func goldenTaskGoal() contract.Goal {
	return contract.Goal{
		Ask: "Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters.",
		Why: "mark the anniversary publicly today.",
		DoneWhen: []contract.DoneLine{
			{Text: "one post is up on the DigiByte account"},
			{Text: "it is under 280 characters and mentions the date", Done: true, ResultID: "r6"},
		},
	}
}

// goldenTaskRules is the rules part of the task example.
func goldenTaskRules() contract.Rules {
	return contract.Rules{
		Corrections: []contract.Correction{
			{ID: "C1", Text: "no, lead with the date not the features"},
		},
		StopWhen: []string{
			"the account shows a login page or a captcha",
			"the post is still over 280 characters after two tries",
		},
	}
}

// goldenTaskWork is the work part of the task example.
func goldenTaskWork() contract.Work {
	return contract.Work{
		Situation: []string{
			`browser tab t1: x.com/compose, "Compose post"`,
			"files changed in this task: none",
		},
		Plan: []contract.PlanStep{
			{Number: 1, Text: "read the product notes", Done: true, ResultID: "r3"},
			{Number: 2, Text: "draft the post", Done: true, ResultID: "r6"},
			{Number: 3, Text: "post it"},
		},
		Results: []contract.ResultLine{
			{ID: "r3", Summary: "read memory/product.md, 2,100 characters"},
			{ID: "r6", Summary: "draft post, 236 characters"},
		},
	}
}

// goldenTaskLessons is the lessons part of the task example.
func goldenTaskLessons() contract.Lessons {
	return contract.Lessons{
		Decisions: []contract.Decision{
			{ID: "D1", Text: "Lead with the date", Reason: "correction C1"},
		},
		Failures: []contract.Failure{
			{ID: "F1", Text: "Draft 1 was 312 characters", Cause: "three facts in one post. Keep to one"},
		},
	}
}

// goldenJobRecord is the job example from the design.
func goldenJobRecord() contract.Record {
	return contract.Record{
		Header: contract.Header{
			Kind:       contract.RecordJob,
			ID:         "4",
			Status:     contract.StatusRunning,
			Origin:     "Signal",
			TasksDone:  3,
			TasksTotal: 12,
			NextDue:    "task 31 today at 14:00",
		},
		Goal:    goldenJobGoal(),
		Rules:   goldenJobRules(),
		Work:    goldenJobWork(),
		Lessons: goldenJobLessons(),
	}
}

// goldenJobGoal is the goal part of the job example.
func goldenJobGoal() contract.Goal {
	return contract.Goal{
		Ask: "Run the DigiByte anniversary campaign this month. One post a day on X, one blog piece, and a summary for me at the end.",
		Why: "keep the anniversary in front of people all month.",
		DoneWhen: []contract.DoneLine{
			{Text: "one post is up for every weekday of the month"},
			{Text: "the blog piece is published"},
			{Text: "the user has the summary"},
		},
	}
}

// goldenJobRules is the rules part of the job example.
func goldenJobRules() contract.Rules {
	return contract.Rules{
		Corrections: []contract.Correction{
			{ID: "C1", Text: "keep every post to one fact"},
		},
		StopWhen: []string{
			"any account shows a login page or a captcha",
			"a post gets more than ten angry replies",
		},
	}
}

// goldenJobWork is the work part of the job example.
func goldenJobWork() contract.Work {
	return contract.Work{
		Situation: []string{"3 of 12 tasks done, none running, next due today at 14:00"},
		Tasks: []contract.JobTask{
			{TaskID: "t17", Text: "post the anniversary tweet", Done: true, ReportID: "j4.1"},
			{TaskID: "t19", Text: "draft the blog piece", Done: true, ReportID: "j4.2"},
			{TaskID: "t22", Text: "post for day two", Done: true, ReportID: "j4.3"},
			{TaskID: "t31", Text: "post for day three", DueAt: "today at 14:00"},
			{TaskID: "t32", Text: "post for day four", DueAt: "tomorrow at 14:00"},
			{TaskID: "t40", Text: "write the summary for the user", DueAt: "after the last post"},
		},
		Results: []contract.ResultLine{
			{ID: "j4.1", Summary: "posted, 236 characters, link saved"},
			{ID: "j4.2", Summary: "draft saved to blog/anniversary.md, 900 words"},
			{ID: "j4.3", Summary: "posted, 198 characters, link saved"},
		},
	}
}

// goldenJobLessons is the lessons part of the job example.
func goldenJobLessons() contract.Lessons {
	return contract.Lessons{
		Decisions: []contract.Decision{
			{ID: "D1", Text: "Post at 14:00 each day", Reason: "the first two posts did best at that hour"},
		},
		Failures: []contract.Failure{
			{ID: "F1", Text: "The day-one post had three facts", Cause: "no rule yet. Now correction C1"},
		},
	}
}
