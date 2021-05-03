# basic-chat
Simple chat server using golongpoll.  Provides a single chat room with HTTP Basic Auth users.

## To Build
```bash
go build basic-chat.go
```

## To Run
```bash
./basic-chat -serve 127.0.0.1:8080 -static ./static/ -accounts "username1:password1,username2:password2,username3:password3"
```

Then visit `http://127.0.0.1:8080` and log in using HTTP basic auth with one of the accounts.

## Configuring
Configure via command-line arguments:

* `-serve` specifies the address to serve HTTP.
* `-static` specifies where the `./static/` directory is located for serving via HTTP. This must be *relative to where the executable is run from* or an absolute path..
* `-saveChatsTo` is the filename to persist chats to. This way chat history is preserved across server restarts.
* `-accounts` is a comma delimited list of `username:password` accounts. Only these users will be allowed to chat.
* `-historySize` is the number of chats to retain per chat room, defaults to 250. This history is present entirely in-memory as well as persisted to disk.

Note: to add a user, simply restart with an updated `-accounts` param.
