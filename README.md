# basic-chat
Simple chat server using golongpoll.  Provides a single chat room with HTTP Basic Auth users.

## To Build
```bash
go build basic-chat.go
```

## To Run
```bash
./basic-chat -serve 127.0.0.1:8080 -clientJs ./static/golongpoll-client.js -accounts "username1:password1,username2:password2,username3:password3"
```

Then visit `http://127.0.0.1:8080` and log in using HTTP basic auth with one of the accounts.

## Configuring
Configure via command-line arguments:

* `-serve` specifies the address to serve HTTP.
* `-clientJs` specifies where the `./static/golongpoll-client.js` file is located for serving via HTTP as a static file. This needs to be the location of the file *relative to where the executable is run from*.
* `-saveChatsTo` is the filename to persist chats to. This way chat history is preserved across server restarts.
* `-accounts` is a comma delimited list of `username:password` accounts. Only these users will be allowed to chat.
* `-historySize` is the number of chats to retain, defaults to 2500. This history is present entirely in-memory as well as persisted to disk.

Note: to add a user, simply restart with an updated `-accounts` param.
