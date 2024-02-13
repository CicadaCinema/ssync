package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sergi/go-diff/diffmatchpatch"
	"simperium-experiments.arseny.uk/ssync"
)

func readSecrets() ssync.Secrets {
	confFile, err := os.Open("conf.json")
	check(err)
	defer confFile.Close()

	var conf ssync.Secrets
	err = json.NewDecoder(confFile).Decode(&conf)
	check(err)

	return conf
}

type Bucket struct {
	currentVersion string
	selectedNoteId string
	entities       map[string]*ssync.Entity
}

func (b *Bucket) Init(bucketChangeVersion string, entities []ssync.Entity) {
	// Populate the bucket.
	b.currentVersion = bucketChangeVersion
	b.entities = make(map[string]*ssync.Entity)
	for i := range entities {
		b.entities[entities[i].Id] = &entities[i]
	}
}

func (b *Bucket) Update(change *ssync.Change) {
	b.entities[change.TargetId].Version = change.EndVersion

	if change.OperationType != "M" || change.OperationValue["content"] == nil {
		return
	}

	content := change.OperationValue["content"].(map[string]interface{})

	if content["o"].(string) != "d" {
		return
	}

	// at this point we know that `content["v"]` contains a diffmatchpatch string

	diffs, err := dmp.DiffFromDelta(b.entities[change.TargetId].NoteContent, content["v"].(string))
	check(err)

	b.entities[change.TargetId].NoteContent = dmp.DiffText2(diffs)
	fmt.Printf("---\n%s\n", b.entities[change.TargetId].NoteContent)

	if b.selectedNoteId == "" {
		b.selectedNoteId = change.TargetId
	}
}

var dmp *diffmatchpatch.DiffMatchPatch

func main() {
	dmp = diffmatchpatch.New()

	bucket := Bucket{}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	messageWrite := make(chan ssync.Change)

	messageWriteTicker := time.NewTicker(time.Second * 30)
	defer messageWriteTicker.Stop()

	go func() {
		for {
			<-messageWriteTicker.C

			if bucket.selectedNoteId == "" {
				continue
			}

			newLine := fmt.Sprintf("I am a Go format string and the current datetime is %s.\n", time.Now().String())
			diff := dmp.DiffMain(bucket.entities[bucket.selectedNoteId].NoteContent, bucket.entities[bucket.selectedNoteId].NoteContent+newLine, false)
			diffString := dmp.DiffToDelta(diff)

			messageWrite <- ssync.Change{
				OperationType: "M",
				OperationValue: map[string]interface{}{
					"content": map[string]interface{}{
						"o": "d",
						"v": diffString,
					},
				},
				TargetId:      bucket.selectedNoteId,
				Ccid:          uuid.New().String(),
				SourceVersion: bucket.entities[bucket.selectedNoteId].Version,
			}
		}
	}()

	var wg sync.WaitGroup
	ssync.StartSyncing(&bucket, readSecrets(), interrupt, messageWrite, &wg)
	fmt.Println("sync started")
	wg.Wait()
	fmt.Println("sync stopped")
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
