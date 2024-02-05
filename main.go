package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sergi/go-diff/diffmatchpatch"
)

var secrets Secrets

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

func request(c *websocket.Conn, command string, messagesToReceive int) ([]string, error) {
	// Write the request.
	err := c.WriteMessage(websocket.TextMessage, []byte(command))
	if err != nil {
		return nil, err
	}

	// Read the response(s).
	messages := make([]string, 0)
	for i := 0; i < messagesToReceive; i++ {
		_, response, err := c.ReadMessage()
		if err != nil {
			return nil, err
		} else {
			messages = append(messages, string(response))
		}
	}
	return messages, nil
}

type EntityMetadata struct {
	Id      string `json:"id"`
	Version int    `json:"v"`
}

type BucketMetadata struct {
	Current string           `json:"current"`
	Index   []EntityMetadata `json:"index"`
}

func main() {
	secrets = readSecrets()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "wss", Host: "api.simperium.com", Path: fmt.Sprintf("/sock/1/%s/websocket", secrets.ApplicationID)}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	messages, err := request(c, fmt.Sprintf(`0:init:{"name":"note","clientid":"simperium-andriod-1.0","api":"1.1","token":"%s","app_id":"%s","library":"simperium-android","version":"1.0"}`, secrets.Token, secrets.ApplicationID), 2)
	if err != nil {
		log.Println(err)
		return
	}

	messages, err = request(c, "0:i::::500", 1)
	if err != nil {
		log.Println(err)
		return
	}

	bucketMetadata := &BucketMetadata{}
	err = json.Unmarshal([]byte(messages[0][4:]), bucketMetadata)
	if err != nil {
		log.Println(err)
		return
	}

	notes := make(map[string]string)
	for _, entity := range bucketMetadata.Index {
		messages, err = request(c, fmt.Sprintf("0:e:%s.%d", entity.Id, entity.Version), 1)
		if err != nil {
			log.Println(err)
			return
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(strings.SplitN(messages[0], "\n", 2)[1]), &data); err != nil {
			panic(err)
		}
		data = data["data"].(map[string]interface{})
		notes[entity.Id] = data["content"].(string)
	}

	done := make(chan struct{})

	dmp := diffmatchpatch.New()

	// reader
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}

			if message[0] == 'h' {
				fmt.Print("h")
			}

			// attempt to update the internal state of `notes` if this is a change command
			// continue if this is not a diffmatchpatch change
			if message[2] == 'c' {
				var changeData map[string]interface{}
				if err := json.Unmarshal([]byte(message[5:len(message)-1]), &changeData); err != nil {
					panic(err)
				}

				if changeData["o"] != "M" {
					continue
				}

				content := changeData["v"].(map[string]interface{})
				content = content["content"].(map[string]interface{})

				if content["o"] != "d" {
					continue
				}

				// at this point we know that `content["v"]` contains a diffmatchpatch string
				log.Printf("recv: %s", message[5:len(message)-1])

				noteId := changeData["id"].(string)

				diffs, err := dmp.DiffFromDelta(notes[noteId], content["v"].(string))
				if err != nil {
					panic(err)
				}

				notes[noteId] = dmp.DiffText2(diffs)
				fmt.Printf("---\n%s\n", notes[noteId])
			}
		}
	}()

	heartbeatTicker := time.NewTicker(time.Second * 20)
	defer heartbeatTicker.Stop()

	// writer
	heartbeatCount := 0
	for {
		select {
		case <-done:
			return
		case <-heartbeatTicker.C:
			err := c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("h:%d", heartbeatCount)))
			// assume heartbeat response was OK
			heartbeatCount += 2
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
