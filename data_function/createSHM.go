package data_function

import (
	"errors"
	"fmt"
	"time"
)

func (mp *DataFunctionManagerProxy) genSHMKey(SHMName string) (int64, error) {
	mp.NameKeyMapMutex.Lock()
	defer mp.NameKeyMapMutex.Unlock()

	_, ok := mp.NameKeyMap[SHMName]
	if ok {
		return -1, errors.New(fmt.Sprintf("SHMName `%s` has exist", SHMName))
	}

	mp.KeyActionMapMutex.Lock()
	defer mp.KeyActionMapMutex.Unlock()

	if int64(len(mp.KeyActionMap)) == ShmKeyMaxCount {
		return -1, errors.New("SHM quantity exceeds maximum limit")
	}
	tryCount := 10
	for true {
		if tryCount == 0 {
			return -1, errors.New("we cannot generate a new Key, means theres too many SHM")
		}
		Key := mp.randNumGenerator.ShmKeyGen()
		_, ok := mp.KeyActionMap[Key]
		if ok {
			Warn("Key %x is exist, we re-generate a New Key", Key)
			tryCount--
			continue
		} else {
			mp.KeyActionMap[Key] = nil
			mp.NameKeyMap[SHMName] = Key
			return Key, nil
		}
	}
	return -1, errors.New("unreachable")
}

func (mp *DataFunctionManagerProxy) CreateSHM(SHMName string, size int) (int64, error) {
	start := time.Now()
	Key, err := mp.genSHMKey(SHMName)
	if err != nil {
		return -1, errors.New(fmt.Sprintf("Error genSHMKey: %s", err))
	}
	Warn("genSHMKey use %d ms", time.Since(start).Milliseconds())

	action, err := mp.actionPool.instantiateAnIdleAction(size)
	if err != nil {
		return -1, errors.New(fmt.Sprintf("Error instantiateAnIdleAction: %s", err))
	}
	Warn("instantiateAnIdleAction use %d ms", time.Since(start).Milliseconds())

	err = action.createSHM(int(Key), size)
	if err != nil {
		return -1, errors.New(fmt.Sprintf("Error invoke Action to create SHM: %s", err))
	}
	Warn("action.createSHM use %d ms", time.Since(start).Milliseconds())

	mp.KeyActionMapMutex.Lock()
	defer mp.KeyActionMapMutex.Unlock()
	mp.KeyActionMap[Key] = action

	return Key, nil
}
