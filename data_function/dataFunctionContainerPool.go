package data_function

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type IdleDataFunctionActionQueue struct {
	IDCounter   atomic.Int64
	capacity    int
	length      atomic.Int32
	actionQueue chan *DataFunctionAction
}

func NewIdleDataFunctionActionQueue(size int) *IdleDataFunctionActionQueue {
	q := &IdleDataFunctionActionQueue{
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
					err := action.createByAPI()
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

func (q *IdleDataFunctionActionQueue) Push(item *DataFunctionAction) {
	q.actionQueue <- item
}

func (q *IdleDataFunctionActionQueue) Pop() *DataFunctionAction {
	q.length.Add(-1)
	return <-q.actionQueue
}

func (q *IdleDataFunctionActionQueue) PopForShared() *DataFunctionAction {
	q.length.Add(-1)
	action := <-q.actionQueue
	action.leftSlots.Store(DefaultSharedDataFunctionMemorySlots)
	action.exclusive = false
	return action
}

type SharedDataFunctionActionList struct {
	mutex       sync.Mutex
	actionIndex map[string]int
	actionList  []*DataFunctionAction
}

func NewSharedDataFunctionActionList() *SharedDataFunctionActionList {
	q := &SharedDataFunctionActionList{
		mutex:       sync.Mutex{},
		actionIndex: make(map[string]int),
		actionList:  []*DataFunctionAction{},
	}
	return q
}

func (l *SharedDataFunctionActionList) Borrow(memory int) *DataFunctionAction {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for _, action := range l.actionList {
		currentSlots := action.leftSlots.Load()
		hopeSLots := currentSlots - int32(memory)
		if hopeSLots >= 0 && action.leftSlots.CompareAndSwap(currentSlots, hopeSLots) {
			return action
		}
	}
	return nil
}

func (l *SharedDataFunctionActionList) Add(action *DataFunctionAction) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if action.leftSlots.Load() != DefaultSharedDataFunctionMemorySlots {
		return errors.New("error to add Action to SharedDataFunctionActionList, whose leftSlots is not equal to the default size")
	}
	index := len(l.actionList)
	l.actionIndex[action.actionName] = index
	l.actionList = append(l.actionList, action)
	return nil
}

func (l *SharedDataFunctionActionList) Back(action *DataFunctionAction, memory int) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	index, ok := l.actionIndex[action.actionName]
	if ok {
		action.leftSlots.Add(int32(memory))
		if len(l.actionList) > 1 && action.leftSlots.Load() == DefaultSharedDataFunctionMemorySlots {
			l.actionList = append(l.actionList[:index], l.actionList[index+1:]...)
			err := action.destroyByAPI()
			if err != nil {
				return errors.New(fmt.Sprintf("error on destroy a shared action `%s`: %s", action.actionName, err))
			}
		}
		return nil
	} else {
		return errors.New(fmt.Sprintf("Error back SHM memory solts, there is not a action named `%s`", action.actionName))
	}
}

type DataFunctionActionPool struct {
	sharedActions *SharedDataFunctionActionList
	idleActions   *IdleDataFunctionActionQueue
	poolSize      int
}

func NewDataFunctionActionPool(poolSize int) (*DataFunctionActionPool, error) {
	pool := &DataFunctionActionPool{
		NewSharedDataFunctionActionList(),
		NewIdleDataFunctionActionQueue(poolSize),
		poolSize,
	}

	err := pool.sharedActions.Add(pool.idleActions.PopForShared())
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error Create sharedActions:%s", err))
	}
	return pool, nil
}

func (pool *DataFunctionActionPool) instantiateAnIdleAction(MiBSizeMemory int) (*DataFunctionAction, error) {
	action := pool.idleActions.Pop()
	// Updating Action memory makes about 3~4 seconds latency, which is Intolerable
	err := action.updateMemByAPI(MiBSizeMemory + BaseMemoryConfigureOfDataFunctionAction)
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

func (pool *DataFunctionActionPool) GetAnSharedAction(MiBSizeMemory int) (*DataFunctionAction, error) {
	action := pool.sharedActions.Borrow(MiBSizeMemory)
	if action != nil {
		return action, nil
	}

	err := pool.sharedActions.Add(pool.idleActions.PopForShared())
	if err != nil {
		errmsg := fmt.Sprintf("Error Add action to sharedActions:%s", err)
		Error(errmsg)
		return nil, errors.New(errmsg)
	}

	action = pool.sharedActions.Borrow(MiBSizeMemory)

	if action != nil {
		return action, nil
	}

	return nil, errors.New("error GetAnSharedAction while Add action to sharedActions")
}
