package worker

import (
	"context"
	"sync"
)

type Worker struct {
	currentJob *Job
	taskChan   chan *Task
	callbacks  Callbacks

	mu sync.Mutex
}

type Callbacks struct {
	TaskStartedCallback   func(ctx context.Context, uuid string)
	TaskFailedCallback    func(ctx context.Context, uuid string, log []byte, err error)
	TaskCompletedCallback func(ctx context.Context, uuid string, log []byte)
}

func NewWorker(taskChan chan *Task, callbacks Callbacks) Interface {
	return &Worker{callbacks: callbacks, taskChan: taskChan}
}

func (q *Worker) Start() {
	for {
		select {
		case task := <-q.taskChan:
			func() {
				job := newJob(task)
				q.setCurrentJob(job)
				defer q.resetCurrentJob()

				q.callbacks.TaskStartedCallback(job.ctx, job.taskUUID)
				if err := job.action(); err != nil {
					q.callbacks.TaskFailedCallback(job.ctx, job.taskUUID, job.Log(), err)
				} else {
					q.callbacks.TaskCompletedCallback(job.ctx, job.taskUUID, job.Log())
				}
			}()
		}
	}
}

func (q *Worker) HoldRunningJobByTaskUUID(uuid string, do func(job *Job)) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.currentJob == nil || q.currentJob.taskUUID != uuid {
		return false
	}

	do(q.currentJob)

	return true
}

func (q *Worker) HasRunningJobByTaskUUID(uuid string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.currentJob != nil && q.currentJob.taskUUID == uuid
}

func (q *Worker) CancelRunningJobByTaskUUID(uuid string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.currentJob != nil && q.currentJob.taskUUID == uuid {
		q.currentJob.ctxCancelFunc()
		return true
	}

	return false
}

func (q *Worker) setCurrentJob(job *Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.currentJob = job
}

func (q *Worker) resetCurrentJob() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.currentJob = nil
}
