package data_function

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const Bytes = 1
const KiB = 1024 * Bytes
const MiB = 1014 * KiB
const GiB = 1024 * MiB

func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}

func sendResult(w http.ResponseWriter, result string) {
	buf := []byte(result)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(buf)))
	numBytesWritten, err := w.Write(buf)

	// flush output
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// diagnostic when you have writing problems
	if err != nil {
		sendError(w, http.StatusInternalServerError, genErrorMessage(fmt.Sprintf("Error writing response: %v", err)))
		return
	}
	if numBytesWritten != len(buf) {
		sendError(w, http.StatusInternalServerError, genErrorMessage(fmt.Sprintf("Only wrote %d of %d bytes to response", numBytesWritten, len(buf))))
		return
	}
}

func sendOK(w http.ResponseWriter) {
	// answer OK
	w.Header().Set("Content-Type", "application/json")
	buf := []byte("{\"ok\":true}\n")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(buf)))
	_, err := w.Write(buf)
	if err != nil {
		log.Fatal(err)
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func sendError(w http.ResponseWriter, code int, cause []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, err := w.Write(cause)
	if err != nil {
		log.Fatal(err)
		return
	}
	_, err = w.Write([]byte("\n"))
	if err != nil {
		log.Fatal(err)
		return
	}
}

type ResponseMessage struct {
	Status  string      `json:"status"`
	Message interface{} `json:"message"`
}

func genErrorMessage(message interface{}) []byte {
	messageResponse := ResponseMessage{Status: "Error", Message: message}
	jsonBytes, _ := json.Marshal(messageResponse)
	return jsonBytes
}

func genOKMessage(message interface{}) string {
	messageResponse := ResponseMessage{Status: "OK", Message: message}
	jsonBytes, _ := json.Marshal(messageResponse)
	return string(jsonBytes)
}

func RESTFUL(method string, url string, requestBody []byte, timeout int) (string, error) {

	// 创建一个不验证HTTPS的Transport
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{
		Timeout:   time.Duration(timeout) * time.Second,
		Transport: tr,
	}

	request, err := http.NewRequest(method, url, bytes.NewBuffer(requestBody))
	if err != nil {
		errMsg := fmt.Sprintf("Error creating request, %s", err)
		Error(errMsg)
		return errMsg, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(AuthName, AuthPassword)

	response, err := client.Do(request)
	if err != nil {
		errMsg := fmt.Sprintf("Error sending request, %s", err)
		Error(errMsg)
		return errMsg, err
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			Error("Error close body, %s", err)
		}
	}(response.Body)

	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(response.Body)
	if err != nil {
		errMsg := fmt.Sprintf("Error reading from response body, %s", err)
		Error(errMsg)
		return errMsg, err
	}

	if response.StatusCode == 200 {
		return responseBody.String(), nil
	} else {
		errMsg := fmt.Sprintf("StatusCode %d, %s", response.StatusCode, responseBody.String())
		return errMsg, errors.New(errMsg)
	}
}

func POSTWithTimeout(url string, requestBody []byte, timeout int) (string, error) {
	return RESTFUL("POST", url, requestBody, timeout)
}

func POST(url string, requestBody []byte) (string, error) {
	return RESTFUL("POST", url, requestBody, 0)
}

func PUT(url string, requestBody []byte) (string, error) {
	return RESTFUL("PUT", url, requestBody, 0)
}

func DELETE(url string) (string, error) {
	return RESTFUL("DELETE", url, []byte{}, 0)
}
