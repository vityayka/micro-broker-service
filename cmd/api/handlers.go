package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type RequestPayload struct {
	Action string      `json:"action"`
	Auth   AuthPayload `json:"auth,omitempty"`
	Log    LogPayload  `json:"log,omitempty"`
}

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LogPayload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func (app *Config) Broker(w http.ResponseWriter, r *http.Request) {
	rsp := jsonResponse{
		Error:   true,
		Message: "Hit	 the broker",
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
		app.logEvent(w, requestPayload.Log)
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
