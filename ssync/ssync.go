package ssync

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Secrets struct {
	ApplicationID string
	Token         string
}

type Entity struct {
	Id          string
	Version     int
	Data        map[string]interface{}
	NoteContent string
}

type Change struct {
	ClientId            string `json:"clientid,omitempty"`
	BucketChangeVersion string `json:"cv,omitempty"`
	SourceVersion       int    `json:"sv,omitempty"`
	EndVersion          int    `json:"ev,omitempty"`
	TargetId            string `json:"id,omitempty"`
	OperationType       string `json:"o,omitempty"`
	// TODO: can the type here be more specific?
	OperationValue map[string]interface{} `json:"v,omitempty"`
	Ccid           string                 `json:"ccid,omitempty"`
	// TODO: can the type here be more specific?
	DataObject map[string]interface{} `json:"d,omitempty"`
}

type InitAndUpdate interface {
	Init(bucketChangeVersion string, entities []Entity)
	Update(change *Change)
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

func createBucket(c *websocket.Conn) (*string, []Entity, error) {
	// request the index
	messages, err := request(c, "0:i::::500", 1)
	if err != nil {
		return nil, nil, err
	}

	var bucketMetadata map[string]interface{}
	err = json.Unmarshal([]byte(messages[0][4:]), &bucketMetadata)
	if err != nil {
		return nil, nil, err
	}

	bucketChangeVersion := bucketMetadata["current"].(string)
	entities := make([]Entity, 0)

	for _, entityMetadata := range bucketMetadata["index"].([]interface{}) {
		entityId := entityMetadata.(map[string]interface{})["id"].(string)
		entityVersion := int(entityMetadata.(map[string]interface{})["v"].(float64))

		// request this entity
		messages, err = request(c, fmt.Sprintf("0:e:%s.%d", entityId, entityVersion), 1)
		if err != nil {
			return nil, nil, err
		}

		var entityData map[string]interface{}
		err := json.Unmarshal([]byte(strings.SplitN(messages[0], "\n", 2)[1]), &entityData)
		if err != nil {
			return nil, nil, err
		}

		entityData = entityData["data"].(map[string]interface{})

		entities = append(entities, Entity{
			Id:          entityId,
			Version:     entityVersion,
			Data:        entityData,
			NoteContent: entityData["content"].(string),
		})
	}

	return &bucketChangeVersion, entities, nil
}

func StartSyncing(state InitAndUpdate, secrets Secrets, interrupt chan os.Signal, writeChange chan Change, doneWg *sync.WaitGroup) {
	doneWg.Add(1)

	u := url.URL{Scheme: "wss", Host: "api.simperium.com", Path: fmt.Sprintf("/sock/1/%s/websocket", secrets.ApplicationID)}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	check(err)

	_, err = request(c, fmt.Sprintf(`0:init:{"name":"note","clientid":"simperium-andriod-1.0","api":"1.1","token":"%s","app_id":"%s","library":"simperium-android","version":"1.0"}`, secrets.Token, secrets.ApplicationID), 2)
	check(err)

	bucketChangeVersion, entities, err := createBucket(c)
	check(err)
	state.Init(*bucketChangeVersion, entities)

	done := make(chan struct{})

	// reader
	doneWg.Add(1)
	go func() {
		defer close(done)
		defer doneWg.Done()

		for {
			_, message, err := c.ReadMessage()
			if err != nil && !websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Println(err)
				break
			}
			check(err)

			if message[0] == 'h' {
				fmt.Print("h")
			}

			// attempt to update the internal state of `notes` if this is a change command
			// continue if this is not a diffmatchpatch change
			if message[2] == 'c' {
				var change Change
				err := json.Unmarshal([]byte(message[5:len(message)-1]), &change)
				check(err)

				state.Update(&change)
			}
		}
	}()

	// writer
	doneWg.Add(1)
	go func() {
		defer doneWg.Done()
		defer c.Close()

		heartbeatTicker := time.NewTicker(time.Second * 20)
		defer heartbeatTicker.Stop()

		heartbeatCount := 0

		for {
			select {
			case <-done:
				return
			case <-heartbeatTicker.C:
				err := c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("h:%d", heartbeatCount)))
				// assume heartbeat response was OK
				heartbeatCount += 2
				check(err)
			case change := <-writeChange:
				changeBytes, err := json.Marshal(change)
				check(err)

				fmt.Printf("0:c:%s", string(changeBytes))
				err = c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("0:c:%s", string(changeBytes))))
				check(err)
			case <-interrupt:
				log.Println("interrupt")

				// Cleanly close the connection by sending a close message and then
				// waiting (with timeout) for the server to close the connection.
				err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				check(err)
				select {
				case <-done:
				case <-time.After(time.Second):
				}
				return
			}
		}
	}()

	doneWg.Done()
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
