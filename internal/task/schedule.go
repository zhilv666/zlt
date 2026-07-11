package task

// ScheduleAction is what a schedule does to its target task when it fires.
type ScheduleAction string

const (
	ScheduleActionStart   ScheduleAction = "start"
	ScheduleActionStop    ScheduleAction = "stop"
	ScheduleActionRestart ScheduleAction = "restart"
)

// Schedule run status values recorded after each execution.
const (
	ScheduleRunSuccess = "success"
	ScheduleRunSkipped = "skipped"
	ScheduleRunFailed  = "failed"
)

// Schedule is a cron plan that applies one action to one registered task.
// Times are RFC3339 strings ("" means never) so the model stays plain data,
// mirroring how Config keeps only JSON-friendly scalar fields.
type Schedule struct {
	ID       string         `json:"id"`
	TaskID   string         `json:"task_id"`
	Name     string         `json:"name"`
	CronExpr string         `json:"cron_expr"`
	Timezone string         `json:"timezone,omitempty"` // IANA name; "" = system local
	Action   ScheduleAction `json:"action"`
	Enabled  bool           `json:"enabled"`

	LastRunAt  string `json:"last_run_at,omitempty"`
	LastStatus string `json:"last_status,omitempty"` // success / skipped / failed
	LastDetail string `json:"last_detail,omitempty"` // reason for skip or error message
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// ScheduleRunResult is the outcome of one schedule execution.
type ScheduleRunResult struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}
