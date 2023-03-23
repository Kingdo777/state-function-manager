package data_function

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const DataFunctionActionCodePath = "/home/kingdo/CLionProjects/DataFunction/src/DataFunction/DataFunction-Virtualenv.zip"
const DataFunctionActionDockerImage = " kingdo/action-python-v3.10"
const DataFunctionActionDockerImageTag = "latest"

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

func (kl *KeepLive) start(dfa *DataFunctionAction) {
	if !kl.running {
		kl.running = true
		go func() {
			for {
				select {
				case <-kl.ticker.C:
					err := dfa.ping()
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

type DataFunctionAction struct {
	ID           int
	namespace    string
	actionName   string
	memConfigure int
	timeout      int
	created      bool
	kl           *KeepLive
	exclusive    bool
}

func NewAction(ID int) *DataFunctionAction {
	actionName := fmt.Sprintf("DataFunction-%d", ID)
	return &DataFunctionAction{
		ID,
		"_",
		actionName,
		256,
		300000,
		false,
		nil,
		true,
	}
}

func (dfa *DataFunctionAction) create() error {
	if !dfa.created {
		dfa.created = true
		createCommand := fmt.Sprintf("%s -i action update %s %s --docker %s:%s -m %d -t %d",
			WskCli,
			dfa.actionName,
			DataFunctionActionCodePath,
			DataFunctionActionDockerImage,
			DataFunctionActionDockerImageTag,
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
		err = dfa.ping()
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

func (dfa *DataFunctionAction) updateMem(newMem int) error {
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

func (dfa *DataFunctionAction) ping() error {

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

func (dfa *DataFunctionAction) destroy() error {
	Debug("Destroy the DataFunction Action: %s", dfa.actionName)
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

func (dfa *DataFunctionAction) createSHM(key int, size int) error {
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

func (dfa *DataFunctionAction) destroySHM(key int64) error {
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
