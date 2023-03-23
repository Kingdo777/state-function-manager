package data_function

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

type DataFunctionActionQueue struct {
	IDCounter   atomic.Int64
	capacity    int
	length      atomic.Int32
	actionQueue chan *DataFunctionAction
}

func NewDataFunctionActionQueue(size int) *DataFunctionActionQueue {
	q := &DataFunctionActionQueue{
		IDCounter:   atomic.Int64{},
		capacity:    size,
		length:      atomic.Int32{},
		actionQueue: make(chan *DataFunctionAction, size),
	}
	q.IDCounter.Store(0)
	q.length.Store(0)
	go func() {
		for true {
			for int(q.length.Add(1)) <= q.capacity {
				go func() {
					Info("Adding a new DataFunctionAction")
					action := NewAction(int(q.IDCounter.Add(1)))
					err := action.create()
					if err != nil {
						Error("Error to add DataFunctionAction: %s", err)
						q.length.Add(-1)
						time.Sleep(10 * time.Second)
						return
					}
					q.Push(action)
					Info("Added new DataFunctionAction : %s", action.actionName)
				}()
			}
			q.length.Add(-1)
			time.Sleep(time.Second)
		}
	}()
	return q
}

func (q *DataFunctionActionQueue) Push(item *DataFunctionAction) {
	q.actionQueue <- item
}

func (q *DataFunctionActionQueue) Pop() *DataFunctionAction {
	q.length.Add(-1)
	return <-q.actionQueue
}

type DataFunctionActionPool struct {
	idleActions *DataFunctionActionQueue
	poolSize    int
}

func NewDataFunctionActionPool(poolSize int) *DataFunctionActionPool {
	return &DataFunctionActionPool{
		NewDataFunctionActionQueue(poolSize),
		poolSize,
	}
}

func (pool *DataFunctionActionPool) instantiateAnIdleAction(memory int) (*DataFunctionAction, error) {

	if memory < 512*MB-64*MB {
		memory = 512 * MB
	} else {
		memory = 512*MB + 64*MB
	}

	action := pool.idleActions.Pop()
	err := action.updateMem(memory)
	if err != nil {
		Error("Error to add instantiate an idle Action: %s", err)
		errDestroy := action.destroy()
		if errDestroy != nil {
			errMsg := fmt.Sprintf("destoy action error: %s; while update action error: %s", errDestroy, err)
			return nil, errors.New(errMsg)
		}
		return nil, err
	}
	return action, nil
}
