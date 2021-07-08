package tasks_manager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/vault/sdk/logical"
	"github.com/werf/vault-plugin-secrets-trdl/pkg/tasks_manager/worker"
)

var BusyError = errors.New("busy")

const (
	taskChanSize = 128

	taskStatusQueued    = "QUEUED"
	taskStatusRunning   = "RUNNING"
	taskStatusCompleted = "COMPLETED"
	taskStatusFailed    = "FAILED"
	taskStatusCanceled  = "CANCELED"
)

type Manager struct {
	Storage logical.Storage
	Worker  worker.Interface

	taskChan chan *worker.Task
	mu       sync.Mutex
}

func NewManager() Interface {
	m := &Manager{taskChan: make(chan *worker.Task, taskChanSize)}
	m.startNewWorker()
	return m
}

func (m *Manager) startNewWorker() {
	m.Worker = worker.NewWorker(m.taskChan, worker.Callbacks{
		TaskStartedCallback:   m.taskStartedCallback,
		TaskFailedCallback:    m.taskFailedCallback,
		TaskCompletedCallback: m.taskCompletedCallback,
	})

	go m.Worker.Start()
}

func (m *Manager) taskStartedCallback(ctx context.Context, uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := markStaleTaskAsFailed(ctx, m.Storage); err != nil {
		return err
	}

	if err := m.Storage.Delete(ctx, storageKeyCurrentRunningTask); err != nil {
		return err
	}

	task, err := getTaskFromStorage(ctx, m.Storage, uuid)
	if err != nil {
		return err
	}

	if task == nil {
		panic(fmt.Sprintf("unexpected error: task %q not found in storage", uuid))
	}

	task.Status = taskStatusRunning
	task.Modified = time.Now()
	if err := putTaskIntoStorage(ctx, m.Storage, task); err != nil {
		return err
	}

	if err := m.Storage.Put(ctx, &logical.StorageEntry{
		Key:   storageKeyCurrentRunningTask,
		Value: []byte(uuid),
	}); err != nil {
		return err
	}

	return nil
}

func (m *Manager) taskCompletedCallback(ctx context.Context, uuid string, log []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.Storage.Delete(ctx, storageKeyCurrentRunningTask); err != nil {
		return err
	}

	task, err := getTaskFromStorage(ctx, m.Storage, uuid)
	if err != nil {
		return err
	}

	if task == nil {
		panic(fmt.Sprintf("unexpected error: task %q not found in storage", uuid))
	}

	task.Status = taskStatusCompleted
	task.Modified = time.Now()
	if err := putTaskIntoStorage(ctx, m.Storage, task); err != nil {
		return err
	}

	if err := m.Storage.Put(ctx, &logical.StorageEntry{
		Key:   taskLogStorageKey(uuid),
		Value: log,
	}); err != nil {
		return err
	}

	return nil
}

func (m *Manager) taskFailedCallback(ctx context.Context, uuid string, log []byte, taskErr error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.Storage.Delete(ctx, storageKeyCurrentRunningTask); err != nil {
		return err
	}

	task, err := getTaskFromStorage(ctx, m.Storage, uuid)
	if err != nil {
		return err
	}

	if task == nil {
		panic(fmt.Sprintf("unexpected error: task %q not found in storage", uuid))
	}

	task.Status = taskStatusFailed
	task.Modified = time.Now()
	if taskErr != nil {
		task.Reason = taskErr.Error()
	}
	if err := putTaskIntoStorage(ctx, m.Storage, task); err != nil {
		return err
	}

	if err := m.Storage.Put(ctx, &logical.StorageEntry{
		Key:   taskLogStorageKey(uuid),
		Value: log,
	}); err != nil {
		return err
	}

	return nil
}
