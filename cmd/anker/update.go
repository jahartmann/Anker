package main

import (
	"anker/internal/updater"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func runUpdate(args []string) error {
	if len(args) != 1 || (args[0] != "status" && args[0] != "check" && args[0] != "install") {
		return errors.New("anker update status | check | install")
	}
	c := updater.UnixClient(updater.Socket, 40*time.Second)
	call := func(method, path string, body io.Reader) (updater.State, error) {
		var state updater.State
		req, err := http.NewRequest(method, "http://updater.local/"+path, body)
		if err != nil {
			return state, err
		}
		req.Header.Set("X-Anker-Request", "1")
		res, err := c.Do(req)
		if err != nil {
			return state, errors.New("Updater nicht erreichbar; anker-updater.service und /etc/anker/update.json prüfen")
		}
		defer res.Body.Close()
		if res.StatusCode >= 400 {
			var v struct {
				Error string `json:"error"`
			}
			json.NewDecoder(res.Body).Decode(&v)
			return state, errors.New(v.Error)
		}
		err = json.NewDecoder(res.Body).Decode(&state)
		return state, err
	}
	action := args[0]
	method := "GET"
	if action != "status" {
		method = "POST"
	}
	var state updater.State
	var err error
	if action == "install" {
		state, err = call("POST", "check", nil)
		if err != nil {
			return err
		}
		if state.Available == nil {
			if state.Status == "check_failed" {
				return errors.New(state.Message)
			}
			fmt.Println("Keine neuere Version verfügbar")
			return nil
		}
		b, _ := json.Marshal(map[string]string{"version": state.Available.Version})
		state, err = call("POST", "install", bytes.NewReader(b))
	} else {
		state, err = call(method, action, nil)
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(state)
}
