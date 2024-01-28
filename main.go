package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

type Secrets struct {
	ApplicationID string
	Token         string
}

func readSecrets() Secrets {
	confFile, err := os.Open("conf.json")
	if err != nil {
		log.Panicf("unable to open configuration file: %s", err)
	}
	defer confFile.Close()

	var conf Secrets
	err = json.NewDecoder(confFile).Decode(&conf)
	if err != nil {
		log.Panicf("unable to decode configuration file: %s", err)
	}

	return conf
}

func main() {
	secrets := readSecrets()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "wss", Host: "api.simperium.com", Path: fmt.Sprintf("/sock/1/%s/websocket", secrets.ApplicationID)}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	err = c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`0:init:{"name":"note","clientid":"simperium-andriod-1.0","api":"1.1","token":"%s","app_id":"%s","library":"simperium-android","version":"1.0"}`, secrets.Token, secrets.ApplicationID)))
	if err != nil {
		log.Println("write:", err)
		return
	}
	_, message, err := c.ReadMessage()
	if err != nil {
		log.Println("read:", err)
		return
	}
	log.Printf("recv: %s", message)
	return

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case t := <-ticker.C:
			err := c.WriteMessage(websocket.TextMessage, []byte(t.String()))
			if err != nil {
				log.Println("write:", err)
				return
			}
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}
