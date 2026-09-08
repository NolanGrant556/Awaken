package scheduledtask

import "time"

// NextRunAt projects the next run instant of a task for the HTTP/Desktop
// list surface (contract [入口与页面](docs/12_定时任务/定时执行与提醒/实现方案.md#入口与页面): the list shows the next run time). It returns
// nil when the task cannot produce another occurrence: paused, completed or
// invalid tasks never arm a timer, and a schedule may simply have no future
// occurrence (one-time calendar in the past awaiting misfire settlement).
// A running task still reports its next future occurrence.
func NextRunAt(task Task, now time.Time) *time.Time {
	if !task.Enabled || task.Completed || task.InvalidReason != "" {
		return nil
	}
	next, ok := NextOccurrence(task.Schedule, now.UTC())
	if !ok {
		return nil
	}
	return &next
}

// LastExecution returns the newest execution-history record of the task, or
// nil when the task has none. It backs the list surface's last_execution
// summary (contract [入口与页面](docs/12_定时任务/定时执行与提醒/实现方案.md#入口与页面): the list shows the latest status and time).
func (s *Service) LastExecution(taskID string) (*Execution, error) {
	executions, err := s.repo.ListExecutions(taskID, 1)
	if err != nil {
		return nil, err
	}
	if len(executions) == 0 {
		return nil, nil
	}
	return &executions[0], nil
}
