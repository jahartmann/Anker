package anker

import "context"

// Job stages are observations from the worker, never estimated percentages.
// Reports are synchronous and belong to the single executing worker.
type JobStage struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	State      string `json:"state"`
	Attempt    int    `json:"attempt"`
	StartedAt  string `json:"started_at"`
	UpdatedAt  string `json:"updated_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	Completed  int    `json:"completed,omitempty"`
	Total      int    `json:"total,omitempty"`
}
type jobProgressKey struct{}
type jobReporter func(string, string, string, int, int) error

func reportJobStage(ctx context.Context, id, title, detail string, completed, total int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if report, ok := ctx.Value(jobProgressKey{}).(jobReporter); ok {
		return report(id, title, detail, completed, total)
	}
	return nil
}
func finishJobStage(j *Job, state string) {
	if len(j.Stages) == 0 {
		return
	}
	stage := &j.Stages[len(j.Stages)-1]
	if stage.State != "running" {
		return
	}
	stage.State, stage.FinishedAt, stage.UpdatedAt = state, now(), now()
}
func (s *Service) jobContext(ctx context.Context, j *Job) context.Context {
	return context.WithValue(ctx, jobProgressKey{}, jobReporter(func(id, title, detail string, completed, total int) error {
		if total < 0 || completed < 0 || completed > total {
			return nil
		}
		at := now()
		if len(j.Stages) == 0 || j.Stages[len(j.Stages)-1].ID != id || j.Stages[len(j.Stages)-1].Attempt != j.Attempts {
			finishJobStage(j, "successful")
			j.Stages = append(j.Stages, JobStage{ID: id, Title: title, Detail: detail, State: "running", Attempt: j.Attempts, StartedAt: at})
		}
		stage := &j.Stages[len(j.Stages)-1]
		stage.Detail, stage.UpdatedAt, stage.Completed, stage.Total = detail, at, completed, total
		return s.saveJobProgress(j)
	}))
}
