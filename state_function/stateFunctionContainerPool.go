package state_function

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type IdleStateFunctionActionQueue struct {
	IDCounter   atomic.Int64
	capacity    int
	length      atomic.Int32
	actionQueue chan *StateFunctionAction
}

func NewIdleStateFunctionActionQueue(size int) *IdleStateFunctionActionQueue {
	q := &IdleStateFunctionActionQueue{
		IDCounter:   atomic.Int64{},
		capacity:    size,
		length:      atomic.Int32{},
		actionQueue: make(chan *StateFunctionAction, size),
	}
	q.IDCounter.Store(0)
	q.length.Store(0)
	go func() {
		for {
			for int(q.length.Add(1)) <= q.capacity {
				//go func() {
				// 顺序创建 StateFunctionAction
				Info("Adding a new StateFunctionAction")
				action := NewAction(int(q.IDCounter.Add(1)))
				for {
					err := action.createByAPI()
					if err != nil {
						Error("Error to add StateFunctionAction: %s, Try again after 10 seconds...", err)
						action.created = false
						time.Sleep(10 * time.Second)
					} else {
						q.Push(action)
						Info("Added new StateFunctionAction : %s", action.actionName)
						break
					}
				}
				//}()
			}
			q.length.Add(-1)
			time.Sleep(time.Second)
		}
	}()
	return q
}

func (q *IdleStateFunctionActionQueue) Push(item *StateFunctionAction) {
	q.actionQueue <- item
}

func (q *IdleStateFunctionActionQueue) Pop() *StateFunctionAction {
	q.length.Add(-1)
	return <-q.actionQueue
}

func (q *IdleStateFunctionActionQueue) PopForShared() *StateFunctionAction {
	q.length.Add(-1)
	action := <-q.actionQueue
	action.leftSlots.Store(DefaultSharedStateFunctionMemorySlots)
	action.exclusive = false
	return action
}

type SharedStateFunctionActionList struct {
	mutex       sync.Mutex
	actionIndex map[string]int
	actionList  []*StateFunctionAction
}

func NewSharedStateFunctionActionList() *SharedStateFunctionActionList {
	q := &SharedStateFunctionActionList{
		mutex:       sync.Mutex{},
		actionIndex: make(map[string]int),
		actionList:  []*StateFunctionAction{},
	}
	return q
}

func (l *SharedStateFunctionActionList) Borrow(memory int) *StateFunctionAction {
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

func (l *SharedStateFunctionActionList) Add(action *StateFunctionAction) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if action.leftSlots.Load() != DefaultSharedStateFunctionMemorySlots {
		return errors.New("error to add Action to SharedStateFunctionActionList, whose leftSlots is not equal to the default size")
	}
	index := len(l.actionList)
	l.actionIndex[action.actionName] = index
	l.actionList = append(l.actionList, action)
	return nil
}

func (l *SharedStateFunctionActionList) Back(action *StateFunctionAction, memory int) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	index, ok := l.actionIndex[action.actionName]
	if ok {
		action.leftSlots.Add(int32(memory))
		if len(l.actionList) > 1 && action.leftSlots.Load() == DefaultSharedStateFunctionMemorySlots {
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

type StateFunctionActionPool struct {
	sharedActions *SharedStateFunctionActionList
	idleActions   *IdleStateFunctionActionQueue
	poolSize      int
}

func NewStateFunctionActionPool(poolSize int) (*StateFunctionActionPool, error) {
	pool := &StateFunctionActionPool{
		NewSharedStateFunctionActionList(),
		NewIdleStateFunctionActionQueue(poolSize),
		poolSize,
	}

	err := pool.sharedActions.Add(pool.idleActions.PopForShared())
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error Create sharedActions:%s", err))
	}
	return pool, nil
}

func (pool *StateFunctionActionPool) instantiateAnIdleAction(MiBSizeMemory int) (*StateFunctionAction, error) {
	action := pool.idleActions.Pop()
	// Updating Action memory makes about 3~4 seconds latency, which is Intolerable
	err := action.updateMemByAPI(MiBSizeMemory + BaseMemoryConfigureOfStateFunctionAction)
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

func (pool *StateFunctionActionPool) GetAnSharedAction(MiBSizeMemory int) (*StateFunctionAction, error) {
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
