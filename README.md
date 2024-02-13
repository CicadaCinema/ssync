`ssync` stands for "Simperium Sync".

At commit `85e3c7b157f00ffea72e251ef24eaf01aeb51924`, some basic proof-of-concept functionality is implemented:
- Populate `conf.json` with your credentials.
- Run `go run main.go`.
- Wait patiently until all notes are fetched from your SimpleNote account. You should wait until you see the letter 'h', incicating a heartbeat.
- Choose any notes and edit them from the SimpleNote web app, your changes should show immediately in the console.
- Observe the client periodically appending messages "I am a Go format string..." to the first note you edited.
- Press CTRL+C for a graceful interrupt.
