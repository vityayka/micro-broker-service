package main

import (
	"broker/event"
	"broker/logs"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net/http"
	"net/rpc"
	"time"
)

type RequestPayload struct {
	Action string      `json:"action"`
	Auth   AuthPayload `json:"auth,omitempty"`
	Log    LogPayload  `json:"log,omitempty"`
	Mail   MailPayload `json:"mail,omitempty"`
}

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type MailPayload struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

type LogPayload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func (app *Config) Broker(w http.ResponseWriter, r *http.Request) {
	rsp := jsonResponse{
		Error:   true,
		Message: "Hit the broker",
	}

	app.writeJSON(w, http.StatusOK, rsp)
}

func (app *Config) HandleSubmission(w http.ResponseWriter, r *http.Request) {
	var requestPayload RequestPayload

	err := app.readJSON(w, r, &requestPayload)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	switch requestPayload.Action {
	case "auth":
		app.authenticate(w, requestPayload.Auth)
		return
	case "log":
		app.logViaRPC(w, requestPayload.Log)
		return
	case "mail":
		app.sendMail(w, requestPayload.Mail)
		return
	default:
		app.errorJson(w, fmt.Errorf("unknown action"))
		return
	}
}

func (app *Config) logEvent(w http.ResponseWriter, event LogPayload) {
	jsonData, _ := json.Marshal(event)
	request, err := http.NewRequest("POST", "http://logger-service", bytes.NewBuffer(jsonData))
	if err != nil {
		app.errorJson(w, err)
		return
	}

	httpClient := &http.Client{}

	rsp, err := httpClient.Do(request)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	if rsp.StatusCode != http.StatusAccepted {
		app.errorJson(w, fmt.Errorf("error connecting logger service"))
		fmt.Printf("response code: %d \n", rsp.StatusCode)
		return
	}

	defer rsp.Body.Close()

	var jsonFromService jsonResponse
	err = json.NewDecoder(rsp.Body).Decode(&jsonFromService)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: jsonFromService.Message,
		Data:    jsonFromService.Data,
	}

	app.writeJSON(w, http.StatusOK, payload)
}

func (app *Config) logViaRPC(w http.ResponseWriter, p LogPayload) {
	client, err := rpc.Dial("tcp", "logger-service:5001")
	if err != nil {
		log.Println("dial err", err)
		app.errorJson(w, err, http.StatusBadGateway)
		return
	}

	var payload = struct {
		Name string
		Data string
	}{Name: p.Name, Data: p.Data}

	var result string

	err = client.Call("RPCServer.NewLog", payload, &result)
	if err != nil {
		log.Println("rpc call err", err)
		app.errorJson(w, err, http.StatusBadGateway)
		return
	}

	var jsonRsp = jsonResponse{
		Error:   false,
		Message: result,
	}

	app.writeJSON(w, http.StatusAccepted, jsonRsp)
}

func (app *Config) pushLogIntoQueue(w http.ResponseWriter, p LogPayload) {
	err := app.pushToQueue(p.Name, p.Data)
	if err != nil {
		log.Println(err)
		app.errorJson(w, err, http.StatusInternalServerError)
		return
	}

	jsonR := jsonResponse{
		Error:   false,
		Message: "log pushed to queue via amqp",
	}

	app.writeJSON(w, http.StatusAccepted, jsonR)
}

func (app *Config) pushToQueue(name, msg string) error {
	emitter, err := event.NewEmitter(app.Rabbit)
	if err != nil {
		return err
	}

	e := LogPayload{
		Name: name,
		Data: msg,
	}

	payload, _ := json.Marshal(e)

	return emitter.Push(string(payload), "log.INFO")
}

func (app *Config) sendMail(w http.ResponseWriter, email MailPayload) {
	jsonData, _ := json.Marshal(email)
	request, err := http.NewRequest("POST", "http://mail-service/send", bytes.NewBuffer(jsonData))
	if err != nil {
		app.errorJson(w, err)
		return
	}

	httpClient := &http.Client{}

	rsp, err := httpClient.Do(request)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	if rsp.StatusCode != http.StatusAccepted {
		app.errorJson(w, fmt.Errorf("error connecting mail service"))
		fmt.Printf("response code: %d \n", rsp.StatusCode)
		return
	}

	defer rsp.Body.Close()

	var jsonFromService jsonResponse
	err = json.NewDecoder(rsp.Body).Decode(&jsonFromService)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: jsonFromService.Message,
		Data:    jsonFromService.Data,
	}

	app.writeJSON(w, http.StatusOK, payload)
}

func (app *Config) authenticate(w http.ResponseWriter, a AuthPayload) {
	jsonData, _ := json.Marshal(a)
	request, err := http.NewRequest("POST", "http://auth-service/auth", bytes.NewBuffer(jsonData))
	if err != nil {
		app.errorJson(w, err)
		return
	}

	httpClient := &http.Client{}

	rsp, err := httpClient.Do(request)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	if rsp.StatusCode == http.StatusUnauthorized {
		app.errorJson(w, fmt.Errorf("invalid creds"), rsp.StatusCode)
		return
	} else if rsp.StatusCode != http.StatusAccepted {
		app.errorJson(w, fmt.Errorf("error connecting auth service"))
		fmt.Printf("response code: %d \n", rsp.StatusCode)
		return
	}

	defer rsp.Body.Close()

	var jsonFromService jsonResponse
	err = json.NewDecoder(rsp.Body).Decode(&jsonFromService)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	if jsonFromService.Error {
		app.errorJson(w, fmt.Errorf(jsonFromService.Message), http.StatusUnauthorized)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: "authenticated",
		Data:    jsonFromService.Data,
	}

	app.writeJSON(w, http.StatusOK, payload)
}

func (app *Config) LogViaGRPC(w http.ResponseWriter, r *http.Request) {
	var jsonRequest RequestPayload
	if err := app.readJSON(w, r, &jsonRequest); err != nil {
		log.Println("error reading json from request")
		app.errorJson(w, err)
		return
	}

	conn, err := grpc.Dial(
		"logger-service:50001",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Println("error dialing logger via gRPC")
		app.errorJson(w, err)
		return
	}
	defer conn.Close()

	c := logs.NewLogServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = c.WriteLog(ctx, &logs.LogRequest{
		LogEntry: &logs.Log{
			Name: jsonRequest.Log.Name,
			Data: jsonRequest.Log.Data,
		},
	})
	if err != nil {
		log.Println("error dialing logger via gRPC")
		app.errorJson(w, err)
		return
	}

	var rsp jsonResponse
	rsp.Error = false
	rsp.Message = "logged"
	app.writeJSON(w, http.StatusAccepted, rsp)
}
