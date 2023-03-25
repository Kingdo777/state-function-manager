package data_function

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type SHMObject struct {
	name string
	// Key mains shm-Key
	Key    int
	size   int
	action *DataFunctionAction
}

func (shm *SHMObject) MibSize() int {
	return ceilDiv(shm.size, MiB)
}

// DataFunctionManagerProxy is the DataFunction Manager running in a separate Action, as a component of Openwhisk
type DataFunctionManagerProxy struct {
	keyGenerator *KeyGenerator

	actionPool *DataFunctionActionPool

	SHMObjectMapMutex sync.Mutex
	SHMObjectMap      map[string]*SHMObject
}

// NewManagerProxy creates a new manager proxy that can handle http requests
func NewManagerProxy() *DataFunctionManagerProxy {

	pool, err := NewDataFunctionActionPool(1)
	if err != nil {
		Error(fmt.Sprintf("Error NewDataFunctionActionPool: %s", err))
		return nil
	}
	return &DataFunctionManagerProxy{
		NewKeyGenerator(FirstShmKey, FirstShmKey+ShmKeyMaxCount),

		pool,

		sync.Mutex{},
		make(map[string]*SHMObject),
	}
}

type CreateSHMResponseMessage struct {
	Key string `json:"key"`
}

type GetSHMResponseMessage struct {
	Key string `json:"key"`
}

func (mp *DataFunctionManagerProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	uri := r.URL.Path

	var requestBody bytes.Buffer
	_, err := requestBody.ReadFrom(r.Body)
	if err != nil {
		errMsg := fmt.Sprintf("Error reading from Response body, %s", err)
		Error(errMsg)
		sendError(w, http.StatusBadRequest, genErrorMessage(errMsg))
		return
	}

	switch uri {
	case "/ping":
		sendOK(w)
		return
	case "/create":
		start := time.Now()
		var createSHMReqBody map[string]string
		err := json.Unmarshal(requestBody.Bytes(), &createSHMReqBody)
		if err != nil {
			sendError(w, http.StatusBadRequest, genErrorMessage(fmt.Sprintf("Error Unmarshal create SHM request body, body:`%s`, err:`%s`", requestBody.String(), err)))
			return
		}
		name, ok := createSHMReqBody["name"]
		sizeS, ok2 := createSHMReqBody["size"]
		if !ok || !ok2 {
			sendError(w, http.StatusBadRequest, genErrorMessage(fmt.Sprintf("Error Unmarshal create SHM request body, must spify filed `name` and `size`, body:`%s`", requestBody.String())))
			return
		}
		size, err := strconv.Atoi(sizeS)
		if err != nil {
			sendError(w, http.StatusBadGateway, genErrorMessage(fmt.Sprintf("size `%s` is cannot conv to int: %s", sizeS, err)))
			return
		}

		Warn("Args check and parse use: %d ms", time.Since(start).Milliseconds())

		Key, err := mp.CreateSHM(name, size)
		if err != nil {
			sendError(w, http.StatusBadGateway, genErrorMessage(fmt.Sprintf("%s", err)))
			return
		}
		Warn("CreateSHM use %d ms", time.Since(start).Milliseconds())
		createSHMResponseMessage := CreateSHMResponseMessage{Key: strconv.FormatInt(int64(Key), 10)}
		sendResult(w, genOKMessage(createSHMResponseMessage))
		return

	case "/get":
		var getSHMReqBody map[string]string
		err := json.Unmarshal(requestBody.Bytes(), &getSHMReqBody)
		if err != nil {
			sendError(w, http.StatusBadRequest, genErrorMessage(fmt.Sprintf("Error Unmarshal get SHM request body, body:`%s`, err:`%s`", requestBody.String(), err)))
			return
		}
		name, ok := getSHMReqBody["name"]
		if !ok {
			sendError(w, http.StatusBadRequest, genErrorMessage(fmt.Sprintf("Error Unmarshal get SHM request body, must spify filed `name`, body:`%s`", requestBody.String())))
			return
		}
		Key, err := mp.GetSHM(name)
		if err != nil {
			sendError(w, http.StatusBadGateway, genErrorMessage(fmt.Sprintf("%s", err)))
			return
		}
		getSHMResponseMessage := GetSHMResponseMessage{Key: strconv.FormatInt(int64(Key), 10)}
		sendResult(w, genOKMessage(getSHMResponseMessage))
		return
	case "/destroy":
		var destroySHMReqBody map[string]string
		err := json.Unmarshal(requestBody.Bytes(), &destroySHMReqBody)
		if err != nil {
			sendError(w, http.StatusBadRequest, genErrorMessage(fmt.Sprintf("Error Unmarshal destroy SHM request body, body:`%s`, err:`%s`", requestBody.String(), err)))
			return
		}
		name, ok := destroySHMReqBody["name"]
		if !ok {
			sendError(w, http.StatusBadRequest, genErrorMessage(fmt.Sprintf("Error Unmarshal destroy SHM request body, must spify filed `name`, body:`%s`", requestBody.String())))
			return
		}
		result, err := mp.DestroySHM(name)
		if err != nil {
			sendError(w, http.StatusBadGateway, genErrorMessage(fmt.Sprintf("%s", err)))
			return
		}
		sendResult(w, genOKMessage(result))
		return
	default:
		errMsg := fmt.Sprintf("Unkonwn Request")
		Error(errMsg)
		sendError(w, http.StatusBadRequest, genErrorMessage(errMsg))
		return
	}
}

// Start creates a proxy to execute actions
func (mp *DataFunctionManagerProxy) Start(port int) {
	// listen and start
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), mp))
}
