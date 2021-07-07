package tasks_manager

import (
	"context"
	"testing"

	"github.com/hashicorp/vault/sdk/logical"
	"github.com/stretchr/testify/assert"
	"github.com/werf/vault-plugin-secrets-trdl/pkg/tasks_manager/worker"
)

// check that Manager.RunTask queues task or returns the busy error
func TestManager_RunTask(t *testing.T) {
	ctx, m, storage := setupTest()

	var uuids []string

	// check the first task
	{
		uuid, err := m.RunTask(ctx, storage, noneTask)
		assert.Nil(t, err)
		assert.NotEmpty(t, uuid)

		assertQueuedTaskInStorage(t, ctx, storage, uuid)

		uuids = append(uuids, uuid)
	}

	// check the second task
	{
		uuid, err := m.RunTask(ctx, storage, noneTask)
		if assert.Error(t, err) {
			assert.Equal(t, err, BusyError)
		}
		assert.Empty(t, uuid)
	}

	// check queue
	for _, uuid := range uuids {
		task := <-m.taskChan
		assert.Equal(t, task.UUID, uuid)
	}
}

// check that Manager.RunTask queues task or returns the busy error
func TestManager_RunTaskWithCurrentRunningTask(t *testing.T) {
	ctx, m, storage := setupTest()

	err := storage.Put(ctx, &logical.StorageEntry{
		Key:   storageKeyCurrentRunningTask,
		Value: []byte("ANY"),
	})
	assert.Nil(t, err)

	{
		uuid, err := m.RunTask(ctx, storage, noneTask)
		if assert.Error(t, err) {
			assert.Equal(t, err, BusyError)
		}
		assert.Empty(t, uuid)
	}

	err = storage.Delete(ctx, storageKeyCurrentRunningTask)
	assert.Nil(t, err)

	{
		uuid, err := m.RunTask(ctx, storage, noneTask)
		assert.Nil(t, err)
		assert.NotEmpty(t, uuid)

		assertQueuedTaskInStorage(t, ctx, storage, uuid)

		task := <-m.taskChan
		assert.Equal(t, task.UUID, uuid)
	}
}

// check that Manager.AddTask queues all tasks
func TestManager_AddTask(t *testing.T) {
	ctx, m, storage := setupTest()

	var uuids []string
	for i := 0; i < 2; i++ {
		uuid, err := m.AddTask(ctx, storage, noneTask)
		assert.Nil(t, err)
		assert.NotEmpty(t, uuid)

		assertQueuedTaskInStorage(t, ctx, storage, uuid)

		uuids = append(uuids, uuid)
	}

	// check queue and task order
	for _, uuid := range uuids {
		task := <-m.taskChan
		assert.Equal(t, task.UUID, uuid)
	}
}

// check that Manager.AddOptionalTask queues task when queue is empty
func TestManager_AddOptionalTask(t *testing.T) {
	ctx, m, storage := setupTest()

	var uuids []string

	// check the first task
	{
		uuid, added, err := m.AddOptionalTask(ctx, storage, noneTask)
		assert.Nil(t, err)
		assert.NotEmpty(t, uuid)
		assert.True(t, added)

		assertQueuedTaskInStorage(t, ctx, storage, uuid)

		uuids = append(uuids, uuid)
	}

	// check the second task
	{
		uuid, added, err := m.AddOptionalTask(ctx, storage, noneTask)
		assert.Nil(t, err)
		assert.Empty(t, uuid)
		assert.False(t, added)
	}

	// check queue
	for _, uuid := range uuids {
		task := <-m.taskChan
		assert.Equal(t, task.UUID, uuid)
	}
}

func setupTest() (context.Context, *Manager, logical.Storage) {
	ctx := context.Background()
	m := initManagerWithoutWorker()
	storage := &logical.InmemStorage{}

	return ctx, m, storage
}

func initManagerWithoutWorker() *Manager {
	taskChan := make(chan *worker.Task, taskChanSize)
	m := &Manager{taskChan: taskChan}
	return m
}

func noneTask(_ context.Context, _ logical.Storage) error { return nil }

func assertQueuedTaskInStorage(t *testing.T, ctx context.Context, storage logical.Storage, uuid string) {
	task, err := getTaskFromStorage(ctx, storage, uuid)
	assert.Nil(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, task.Status, taskStatusQueued)
}
