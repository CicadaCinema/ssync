package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
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

type SyncedFile struct {
	FilePath    string
	FileContent []byte
}

type Bucket struct {
	pathToId map[string]string
	idToPath map[string]string
	// do not run `Update` in response to local changes
	ignoreNextUpdate bool
	// do not respond to sync file update events as if they were local
	ignoreNextFileEvent bool
}

func (b *Bucket) Init(bucketChangeVersion string, entities []ssync.Entity) {
	// Populate the bucket.
	b.pathToId = make(map[string]string)
	b.idToPath = make(map[string]string)

	for i := range entities {
		var file SyncedFile
		err := json.Unmarshal([]byte(entities[i].NoteContent), &file)
		check(err)

		err = os.WriteFile(file.FilePath, file.FileContent, 0644)
		check(err)

		b.pathToId[file.FilePath] = entities[i].Id
		b.idToPath[entities[i].Id] = file.FilePath
	}
}

func (b *Bucket) Update(change *ssync.Change) {
	if b.ignoreNextUpdate {
		b.ignoreNextUpdate = false
		return
	}

	b.ignoreNextFileEvent = true

	switch change.OperationType {
	case "M":
		var file SyncedFile
		err := json.Unmarshal([]byte(change.OperationValue["content"].(map[string]interface{})["v"].(string)), &file)
		check(err)

		err = os.WriteFile(file.FilePath, file.FileContent, 0644)
		check(err)

		b.pathToId[file.FilePath] = change.TargetId
		b.idToPath[change.TargetId] = file.FilePath
	case "-":
		err := os.Remove(b.idToPath[change.TargetId])
		check(err)

		delete(b.pathToId, b.idToPath[change.TargetId])
		delete(b.idToPath, change.TargetId)
	default:
		fmt.Println("Invalid change.OperationType .")
		panic(change)
	}
}

func main() {
	syncDirPath := "SyncDir"

	err := os.Mkdir(syncDirPath, 0755)
	check(err)
	defer os.RemoveAll(syncDirPath)

	bucket := Bucket{}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	messageWrite := make(chan ssync.Change)

	watcher, err := fsnotify.NewWatcher()
	check(err)
	defer watcher.Close()

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Has(fsnotify.Write) {
					fmt.Println("write:", event.Name)

					if bucket.ignoreNextFileEvent {
						bucket.ignoreNextFileEvent = false
						continue
					}

					bucket.ignoreNextUpdate = true

					entityId := uuid.New().String()
					bucket.pathToId[event.Name] = entityId
					bucket.idToPath[entityId] = event.Name

					content, err := os.ReadFile(event.Name)

					file := SyncedFile{
						FilePath:    event.Name,
						FileContent: content,
					}
					fileString, err := json.Marshal(file)
					check(err)

					messageWrite <- ssync.Change{
						//BucketChangeVersion: bucket.currentVersion,
						OperationType: "M",
						OperationValue: map[string]interface{}{
							"content": map[string]interface{}{
								"o": "+",
								"v": string(fileString),
							},
							// the below is required for the Simplenote server to accept this entity
							// turns out that only entities which are already in the trash can be deleted,
							// so we create this entity as already-trashed
							"creationDate": map[string]interface{}{
								"o": "+",
								"v": 0,
							},
							"modificationDate": map[string]interface{}{
								"o": "+",
								"v": 0,
							},
							"deleted": map[string]interface{}{
								"o": "+",
								"v": true,
							},
							"publishURL": map[string]interface{}{
								"o": "+",
								"v": "",
							},
							"shareURL": map[string]interface{}{
								"o": "+",
								"v": "",
							},
							"systemTags": map[string]interface{}{
								"o": "+",
								"v": []interface{}{},
							},
							"tags": map[string]interface{}{
								"o": "+",
								"v": []interface{}{},
							},
						},
						TargetId: entityId,
						Ccid:     uuid.New().String(),
					}
				} else if event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
					fmt.Println("delete:", event.Name)

					if bucket.ignoreNextFileEvent {
						bucket.ignoreNextFileEvent = false
						continue
					}

					bucket.ignoreNextUpdate = true

					messageWrite <- ssync.Change{
						OperationType: "-",
						TargetId:      bucket.pathToId[event.Name],
						Ccid:          uuid.New().String(),
					}

					delete(bucket.idToPath, bucket.pathToId[event.Name])
					delete(bucket.pathToId, event.Name)
				} else {
					continue
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("error:", err)
			}
		}
	}()

	var wg sync.WaitGroup
	ssync.StartSyncing(&bucket, readSecrets(), interrupt, messageWrite, &wg)

	// only start watching the directory after we have fetched any remote files
	err = watcher.Add(syncDirPath)
	check(err)

	fmt.Println("sync started")
	wg.Wait()
	fmt.Println("sync stopped")
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
