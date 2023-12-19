package state_function

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

func StateFunctionActionCodePath() string {
	var value, ok = os.LookupEnv("StateFunctionActionCodePath")
	if !ok {
		value = "/home/kingdo/CLionProjects/chestbox/StateFunction/action/src/state_function_action/__main__.py"
	}
	return value
}

const StateFunctionActionDockerImage = "kingdo/action-python-v3.10"
const StateFunctionActionDockerImageTag = "latest"

const BaseMemoryConfigureOfStateFunctionAction = 64
const DefaultSharedStateFunctionMemorySlots = 512

type KeepLive struct {
	ticker   *time.Ticker
	stopChan chan bool
	running  bool
}

func CreateKeepLive(interval int) *KeepLive {
	return &KeepLive{
		time.NewTicker(time.Duration(interval) * time.Second),
		make(chan bool),
		false,
	}
}

func (kl *KeepLive) start(dfa *StateFunctionAction) {
	if !kl.running {
		kl.running = true
		go func() {
			for {
				select {
				case <-kl.ticker.C:
					err := dfa.pingByAPI()
					if err != nil {
						Warn("Keep live error, cannot ping the action `%s`", dfa.actionName)
						return
					}
				case <-kl.stopChan:
					kl.ticker.Stop()
					return
				}
			}
		}()
	}
}

func (kl *KeepLive) stop() {
	if kl.running {
		kl.stopChan <- true
		kl.running = false
	}
}

type StateFunctionAction struct {
	ID           int
	namespace    string
	actionName   string
	memConfigure int
	timeout      int
	created      bool
	kl           *KeepLive
	leftSlots    atomic.Int32
	exclusive    bool
}

func NewAction(ID int) *StateFunctionAction {
	actionName := fmt.Sprintf("StateFunction-%d", ID)
	return &StateFunctionAction{
		ID,
		"guest",
		actionName,
		DefaultSharedStateFunctionMemorySlots + BaseMemoryConfigureOfStateFunctionAction,
		300000,
		false,
		nil,
		atomic.Int32{},
		true,
	}
}

type UpdateActionBody struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Exec      struct {
		Kind   string `json:"kind"`
		Code   []byte `json:"code"`
		Image  string `json:"image"`
		Binary bool   `json:"binary"`
	} `json:"exec"`
	Annotations []struct {
		Key   string      `json:"key"`
		Value interface{} `json:"value"`
	} `json:"annotations"`
	Limits struct {
		Timeout     int `json:"timeout"`
		Memory      int `json:"memory"`
		Logs        int `json:"logs"`
		Concurrency int `json:"concurrency"`
	} `json:"limits"`
	Publish bool `json:"publish"`
}

func (dfa *StateFunctionAction) updateByAPI(ping bool) error {

	startTime := time.Now()

	code, _ := os.ReadFile(StateFunctionActionCodePath())

	body := UpdateActionBody{
		Namespace: dfa.namespace,
		Name:      dfa.actionName,
		Exec: struct {
			Kind   string `json:"kind"`
			Code   []byte `json:"code"`
			Image  string `json:"image"`
			Binary bool   `json:"binary"`
		}{
			Kind:   "blackbox",
			Code:   code,
			Image:  fmt.Sprintf("%s:%s", StateFunctionActionDockerImage, StateFunctionActionDockerImageTag),
			Binary: false,
		},
		Annotations: []struct {
			Key   string      `json:"key"`
			Value interface{} `json:"value"`
		}{
			{
				Key:   "provide-api-key",
				Value: false,
			},
			{
				Key:   "exec",
				Value: "blackbox",
			},
		},
		Limits: struct {
			Timeout     int `json:"timeout"`
			Memory      int `json:"memory"`
			Logs        int `json:"logs"`
			Concurrency int `json:"concurrency"`
		}{
			Timeout:     dfa.timeout,
			Memory:      dfa.memConfigure,
			Logs:        10,
			Concurrency: 1,
		},
		Publish: false,
	}

	url := fmt.Sprintf("https://%s/api/v1/namespaces/%s/actions/%s?overwrite=true", ApiHost(), dfa.namespace, dfa.actionName)
	Info(url)

	param, _ := json.Marshal(body)

	_, err := PUT(url, param)
	if err != nil {
		Error("invoke updateByAPI Error, %s", err)
		return err
	}
	Debug("Update StateFunction Action: %s, used %d ms", dfa.actionName, time.Since(startTime).Milliseconds())

	if ping {
		startTime = time.Now()

		err = dfa.pingByAPI()
		if err != nil {
			dfa.created = false
			return err
		}
		dfa.kl = CreateKeepLive(15)
		dfa.kl.start(dfa)

		Debug("Ping the New-Created StateFunction Action: %s, used %d ms", dfa.actionName, time.Since(startTime).Milliseconds())
		return nil
	}
	return errors.New(fmt.Sprintf("Action `%s` has been created", dfa.actionName))
}

func (dfa *StateFunctionAction) createByAPI() error {
	if !dfa.created {
		dfa.created = true
		return dfa.updateByAPI(true)
	}
	return errors.New(fmt.Sprintf("Action `%s` has been created", dfa.actionName))
}

func (dfa *StateFunctionAction) create() error {
	if !dfa.created {
		dfa.created = true
		createCommand := fmt.Sprintf("%s -i action update %s %s --docker %s:%s -m %d -t %d",
			WskCli,
			dfa.actionName,
			StateFunctionActionCodePath(),
			StateFunctionActionDockerImage,
			StateFunctionActionDockerImageTag,
			dfa.memConfigure,
			dfa.timeout,
		)
		Debug(createCommand)
		cmd := exec.Command("sh", "-c", createCommand)
		cmd.Env = append(os.Environ(), fmt.Sprintf("WSK_CONFIG_FILE=%s", WskConfigFile))
		var outBuffer bytes.Buffer
		cmd.Stdout = &outBuffer
		err := cmd.Start()
		if err != nil {
			dfa.created = false
			Error("createCommand Start Error, %s", err)
			return err
		}
		err = cmd.Wait()
		if err != nil {
			dfa.created = false
			Error("createCommand Wait Error, %s", err)
			return err
		}
		err = dfa.pingByAPI()
		if err != nil {
			dfa.created = false
			return err
		}
		dfa.kl = CreateKeepLive(15)
		dfa.kl.start(dfa)
		return nil
	}
	return errors.New(fmt.Sprintf("Action `%s` has been created", dfa.actionName))
}

func (dfa *StateFunctionAction) updateMemByAPI(newMem int) error {
	if !dfa.created {
		return errors.New(fmt.Sprintf("Action `%s` is not created", dfa.actionName))
	}

	if dfa.memConfigure != newMem {
		dfa.memConfigure = newMem
	} else {
		return nil
	}

	return dfa.updateByAPI(false)
}

func (dfa *StateFunctionAction) updateMem(newMem int) error {
	if !dfa.created {
		return errors.New(fmt.Sprintf("Action `%s` is not created", dfa.actionName))
	}

	if dfa.memConfigure != newMem {
		dfa.memConfigure = newMem
	} else {
		return nil
	}

	updateCommand := fmt.Sprintf("%s -i action update %s -m %d",
		WskCli,
		dfa.actionName,
		dfa.memConfigure,
	)
	Debug(updateCommand)
	cmd := exec.Command("sh", "-c", updateCommand)
	cmd.Env = append(os.Environ(), fmt.Sprintf("WSK_CONFIG_FILE=%s", WskConfigFile))
	var outBuffer bytes.Buffer
	cmd.Stderr = &outBuffer
	err := cmd.Start()
	if err != nil {
		Error("updateCommand Start Error, %s, %s", err, outBuffer.String())
		return err
	}
	err = cmd.Wait()
	if err != nil {
		Error("updateCommand Wait Error, %s, %s", err, outBuffer.String())
		return err
	}
	Debug("updateMem of %s Success", dfa.actionName)
	return nil
}

type pingActionParam struct {
	Op string `json:"op"`
}

func (dfa *StateFunctionAction) pingByAPI() error {

	//curl -X 'POST' \
	//'https://raw.githubusercontent.com/api/v1/namespaces/guest/actions/StateFunction-1?blocking=true&result=true' \
	//-H 'accept: application/json' \
	//-H 'authorization: Basic MjNiYzQ2YjEtNzFmNi00ZWQ1LThjNTQtODE2YWE0ZjhjNTAyOjEyM3pPM3haQ0xyTU42djJCS0sxZFhZRnBYbFBrY2NPRnFtMTJDZEFzTWdSVTRWck5aOWx5R1ZDR3VNREdJd1A=' \
	//-H 'Content-Type: application/json' \
	//-d '{"op":"ping"}'

	url := fmt.Sprintf("https://%s/api/v1/namespaces/%s/actions/%s?blocking=true&result=true", ApiHost(), dfa.namespace, dfa.actionName)

	param, _ := json.Marshal(pingActionParam{
		"ping",
	})
	out, err := POSTWithTimeout(url, param, 60)
	if err != nil {
		Error("pingByAPI Error, %s", err)
		return err
	}

	var actionResult map[string]string
	err = json.Unmarshal([]byte(out), &actionResult)
	if err != nil {
		errMsg := fmt.Sprintf(
			"Error Unmarshal action body, body:`%s`, err:`%s`",
			out, err,
		)
		Error(errMsg)
		return errors.New(errMsg)
	}
	if actionResult["body"] == "PONG" && actionResult["statusCode"] == "200" {
		Info("Ping Action `%s` success", dfa.actionName)
		return nil
	} else {
		errMsg := fmt.Sprintf("Un-pong, result: %s", out)
		Error(errMsg)
		return errors.New(errMsg)
	}
}

func (dfa *StateFunctionAction) ping() error {

	updateCommand := fmt.Sprintf("%s -i action invoke --result --blocking %s --param op ping",
		WskCli,
		dfa.actionName,
	)

	cmd := exec.Command("sh", "-c", updateCommand)
	cmd.Env = append(os.Environ(), fmt.Sprintf("WSK_CONFIG_FILE=%s", WskConfigFile))
	var outBuffer bytes.Buffer
	cmd.Stdout = &outBuffer
	err := cmd.Start()
	if err != nil {
		Error("updateCommand Start Error, %s", err)
		return err
	}
	cmdError := make(chan error)
	pingSuccess := make(chan bool)

	timer := time.NewTimer(time.Minute)
	go func() {
		err = cmd.Wait()
		if err != nil {
			Error("updateCommand Start Error, %s", err)
			cmdError <- err
		}

		var actionResult map[string]string
		err := json.Unmarshal(outBuffer.Bytes(), &actionResult)
		if err != nil {
			errMsg := fmt.Sprintf(
				"Error Unmarshal action body, body:`%s`, err:`%s`",
				outBuffer.String(), err,
			)
			Error(errMsg)
			cmdError <- errors.New(errMsg)
		}
		if actionResult["body"] == "PONG" && actionResult["statusCode"] == "200" {
			Info("Ping Action `%s` success", dfa.actionName)
			pingSuccess <- true
		} else {
			errMsg := fmt.Sprintf("Un-pong, result: %s", outBuffer.String())
			Error(errMsg)
			cmdError <- errors.New(errMsg)
		}
		return
	}()

	select {
	case <-timer.C:
		errMsg := fmt.Sprintf("Timeout while ping Action `%s`", dfa.actionName)
		Error(errMsg)
		return errors.New(errMsg)
	case <-pingSuccess:
		return nil
	case err := <-cmdError:
		return err
	}
}

func (dfa *StateFunctionAction) destroyByAPI() error {
	if !dfa.created {
		return errors.New(fmt.Sprintf("Action `%s` is not created", dfa.actionName))
	}
	startTime := time.Now()
	dfa.kl.stop()
	dfa.kl = nil
	dfa.created = false

	//curl -X 'DELETE' \
	//'https://raw.githubusercontent.com/api/v1/namespaces/guest/actions/actionName' \
	//-H 'accept: application/json' \
	//-H 'authorization: Basic MjNiYzQ2YjEtNzFmNi00ZWQ1LThjNTQtODE2YWE0ZjhjNTAyOjEyM3pPM3haQ0xyTU42djJCS0sxZFhZRnBYbFBrY2NPRnFtMTJDZEFzTWdSVTRWck5aOWx5R1ZDR3VNREdJd1A='

	url := fmt.Sprintf("https://%s/api/v1/namespaces/%s/actions/%s", ApiHost(), dfa.namespace, dfa.actionName)

	_, err := DELETE(url)
	if err != nil {
		Error("invoke destroyByAPI Error, %s", err)
		return err
	}
	Debug("Destroy the StateFunction Action: %s, used %d ms", dfa.actionName, time.Since(startTime).Milliseconds())
	return nil
}

func (dfa *StateFunctionAction) destroy() error {
	Debug("Destroy the StateFunction Action: %s", dfa.actionName)
	if !dfa.created {
		return errors.New(fmt.Sprintf("Action `%s` is not created", dfa.actionName))
	}

	dfa.kl.stop()
	dfa.kl = nil
	dfa.created = false

	deleteCommand := fmt.Sprintf("%s -i action delete %s ",
		WskCli,
		dfa.actionName)

	cmd := exec.Command("sh", "-c", deleteCommand)
	cmd.Env = append(os.Environ(), fmt.Sprintf("WSK_CONFIG_FILE=%s", WskConfigFile))
	err := cmd.Start()
	if err != nil {
		Error("deleteCommand Start Error, %s", err)
		return err
	}
	err = cmd.Wait()
	if err != nil {
		Error("deleteCommand Wait Error, %s", err)
		return err
	}

	return nil
}

type CreateSHMParam struct {
	Op   string `json:"op"`
	Key  int    `json:"key"`
	Size int    `json:"size"`
}

// docs: https://github.com/apache/openwhisk/blob/master/docs/rest_api.md#using-rest-apis-with-openwhisk
func (dfa *StateFunctionAction) createSHMbyAPI(key int, size int) error {
	if !dfa.created {
		return errors.New(fmt.Sprintf("Action `%s` is not created", dfa.actionName))
	}

	//curl -X 'POST' \
	//'https://raw.githubusercontent.com/api/v1/namespaces/guest/actions/StateFunction-1?blocking=true&result=true' \
	//-H 'accept: application/json' \
	//-H 'authorization: Basic MjNiYzQ2YjEtNzFmNi00ZWQ1LThjNTQtODE2YWE0ZjhjNTAyOjEyM3pPM3haQ0xyTU42djJCS0sxZFhZRnBYbFBrY2NPRnFtMTJDZEFzTWdSVTRWck5aOWx5R1ZDR3VNREdJd1A=' \
	//-H 'Content-Type: application/json' \
	//-d '{"op":"ping"}'

	url := fmt.Sprintf("https://%s/api/v1/namespaces/%s/actions/%s?blocking=true&result=true", ApiHost(), dfa.namespace, dfa.actionName)

	param, _ := json.Marshal(CreateSHMParam{
		"create",
		key,
		size,
	})
	out, err := POST(url, param)
	if err != nil {
		Error("invoke createSHMbyAPI Error, %s", err)
		return err
	}
	Debug("createSHM: %s", strings.Replace(out, "\n", "", -1))
	return nil
}

func (dfa *StateFunctionAction) createSHM(key int, size int) error {
	if !dfa.created {
		return errors.New(fmt.Sprintf("Action `%s` is not created", dfa.actionName))
	}
	invokeCommand := fmt.Sprintf("%s -i action invoke --result --blocking %s "+
		"--param op create "+
		"--param key %d "+
		"--param size %d ",
		WskCli,
		dfa.actionName,
		key,
		size,
	)
	Debug(invokeCommand)
	cmd := exec.Command("sh", "-c", invokeCommand)
	cmd.Env = append(os.Environ(), fmt.Sprintf("WSK_CONFIG_FILE=%s", WskConfigFile))
	var outBuffer bytes.Buffer
	cmd.Stdout = &outBuffer
	err := cmd.Start()
	if err != nil {
		Error("invokeCommand Start Error, %s", err)
		return err
	}
	err = cmd.Wait()
	if err != nil {
		Error("invokeCommand Wait Error, %s", err)
		return err
	}
	Debug("createSHM: %s", strings.Replace(outBuffer.String(), "\n", "", -1))
	return nil
}

type DeleteSHMParam struct {
	Op  string `json:"op"`
	Key int    `json:"key"`
}

func (dfa *StateFunctionAction) destroySHMbyAPI(key int) error {
	if !dfa.created {
		return errors.New(fmt.Sprintf("Action `%s` is not created", dfa.actionName))
	}

	//curl -X 'POST' \
	//'https://raw.githubusercontent.com/api/v1/namespaces/guest/actions/StateFunction-1?blocking=true&result=true' \
	//-H 'accept: application/json' \
	//-H 'authorization: Basic MjNiYzQ2YjEtNzFmNi00ZWQ1LThjNTQtODE2YWE0ZjhjNTAyOjEyM3pPM3haQ0xyTU42djJCS0sxZFhZRnBYbFBrY2NPRnFtMTJDZEFzTWdSVTRWck5aOWx5R1ZDR3VNREdJd1A=' \
	//-H 'Content-Type: application/json' \
	//-d '{"op":"ping"}'

	url := fmt.Sprintf("https://%s/api/v1/namespaces/%s/actions/%s?blocking=true&result=true", ApiHost(), dfa.namespace, dfa.actionName)

	param, _ := json.Marshal(DeleteSHMParam{
		"destroy",
		key,
	})
	out, err := POST(url, param)
	if err != nil {
		Error("invoke destroySHMbyAPI Error, %s", err)
		return err
	}
	Debug("destroySHM: %s", strings.Replace(out, "\n", "", -1))
	return nil
}
func (dfa *StateFunctionAction) destroySHM(key int) error {
	if !dfa.created {
		return errors.New(fmt.Sprintf("Action `%s` is not created", dfa.actionName))
	}
	invokeCommand := fmt.Sprintf("%s -i action invoke --result --blocking %s "+
		"--param op destroy "+
		"--param key %d ",
		WskCli,
		dfa.actionName,
		key,
	)
	cmd := exec.Command("sh", "-c", invokeCommand)
	cmd.Env = append(os.Environ(), fmt.Sprintf("WSK_CONFIG_FILE=%s", WskConfigFile))
	var outBuffer bytes.Buffer
	cmd.Stdout = &outBuffer
	err := cmd.Start()
	if err != nil {
		Error("invokeCommand Start Error, %s", err)
		return err
	}
	err = cmd.Wait()
	if err != nil {
		Error("invokeCommand Wait Error, %s", err)
		return err
	}
	Debug("destroySHM: %s", strings.Replace(outBuffer.String(), "\n", "", -1))
	return nil
}
