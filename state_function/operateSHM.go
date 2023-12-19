package state_function

import (
	"errors"
	"fmt"
	"time"
)

func (mp *StateFunctionManagerProxy) CreateSHM(SHMName string, bytesSize int) (int, error) {
	start := time.Now()
	Key, ok := mp.keyGenerator.GetKey()
	if !ok {
		return -1, errors.New(fmt.Sprintf("Error genSHMKey"))
	}

	MiBSizeMemory := ceilDiv(bytesSize, MiB)

	var action *StateFunctionAction
	var err error

	if MiBSizeMemory > 256*MiB {
		action, err = mp.actionPool.instantiateAnIdleAction(MiBSizeMemory)
		if err != nil {
			return -1, errors.New(fmt.Sprintf("Error instantiateAnIdleAction: %s", err))
		}
		Warn("instantiateAnIdleAction use %d ms", time.Since(start).Milliseconds())
	} else {
		action, err = mp.actionPool.GetAnSharedAction(MiBSizeMemory)
		if err != nil {
			return -1, errors.New(fmt.Sprintf("Error GetAnSharedAction: %s", err))
		}
		Warn("GetAnSharedAction use %d ms", time.Since(start).Milliseconds())
	}

	err = action.createSHMbyAPI(Key, bytesSize)
	if err != nil {
		return -1, errors.New(fmt.Sprintf("Error invoke Action to create SHM: %s", err))
	}
	Warn("action.createSHM use %d ms", time.Since(start).Milliseconds())

	mp.SHMObjectMapMutex.Lock()
	defer mp.SHMObjectMapMutex.Unlock()
	mp.SHMObjectMap[SHMName] = &SHMObject{
		SHMName,
		Key,
		bytesSize,
		action,
	}
	return Key, nil
}

func (mp *StateFunctionManagerProxy) GetSHM(name string) (int, error) {
	mp.SHMObjectMapMutex.Lock()
	defer mp.SHMObjectMapMutex.Unlock()

	shm, ok := mp.SHMObjectMap[name]
	if !ok {
		return -1, errors.New(fmt.Sprintf("cannot find Key of name: `%s`", name))
	}

	return shm.Key, nil
}

func (mp *StateFunctionManagerProxy) DestroySHM(name string) (string, error) {
	mp.SHMObjectMapMutex.Lock()

	shm, ok := mp.SHMObjectMap[name]
	if !ok {
		mp.SHMObjectMapMutex.Unlock()
		return "", errors.New(fmt.Sprintf("cannot find Key of name: `%s`", name))
	}
	mp.SHMObjectMapMutex.Unlock()

	action := shm.action
	err := action.destroySHMbyAPI(shm.Key)
	if err != nil {
		return "", errors.New(fmt.Sprintf("error on exec action.destroySHM(): %s", err))
	}

	if !action.exclusive {
		err := mp.actionPool.sharedActions.Back(action, shm.MibSize())
		if err != nil {
			return "", errors.New(fmt.Sprintf("error on Back released SHM slots to SharedAction: %s", err))
		}
	} else {
		err := action.destroyByAPI()
		if err != nil {
			return "", errors.New(fmt.Sprintf("error on destroy an exclusive action `%s`: %s", action.actionName, err))
		}
	}

	mp.SHMObjectMapMutex.Lock()
	defer mp.SHMObjectMapMutex.Unlock()
	mp.keyGenerator.ReturnKey(shm.Key)
	delete(mp.SHMObjectMap, name)

	return fmt.Sprintf("DestroySHM %s Success", name), nil
}
