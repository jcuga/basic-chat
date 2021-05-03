package main

import (
	"crypto/subtle"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcuga/golongpoll"
)

type User struct {
	Username string
	Password string
}

func main() {
	serveAddr := flag.String("serve", "127.0.0.1:8080", "Address to serve HTTP on.")
	staticDir := flag.String("static", "./static/", "where the static directory is located relative to where this binary runs.")
	persistFilename := flag.String("saveChatsTo", "./chats.json", "Where to save chat history so not lost when server is restarted.")
	usersList := flag.String("accounts", "", "comma delimited user:pwd for basic auth accounts.")
	chatHistorySize := flag.Uint("-historySize", 250, "Max number of chats in history per chat room.")

	flag.Parse()

	filePersistor, err := golongpoll.NewFilePersistor(*persistFilename, 4096, 2)
	if err != nil {
		fmt.Printf("Failed to create file persistor, error: %v", err)
		return
	}

	users := parseAccounts(*usersList)
	if len(users) == 0 {
		fmt.Printf("Must provide at least one account (user:password) via -accounts flag")
		os.Exit(1)
	}
	if *chatHistorySize < 10 {
		fmt.Printf("-historySize must be >= 10, got: %d", *chatHistorySize)
		os.Exit(1)
	}

	manager, err := golongpoll.StartLongpoll(golongpoll.Options{
		// hang on to N most recent chats:
		MaxEventBufferSize: int(*chatHistorySize),
		AddOn:              filePersistor,
	})
	if err != nil {
		log.Fatalf("Failed to create chat longpoll manager: %q\n", err)
	}

	// web app pages
	http.HandleFunc("/", requireBasicAuth(indexPage, users))
	http.HandleFunc("/chat", requireBasicAuth(chatroomPage, users))
	http.HandleFunc("/logout", requireBasicAuth(logoutPage, users))

	// longpoll pub-sub api:
	http.HandleFunc("/publish", requireBasicAuth(manager.PublishHandler, users))
	http.HandleFunc("/events", requireBasicAuth(manager.SubscriptionHandler, users))

	// Serve static files--doing files explicitly instead of http.FileServer so
	// we can a) be explicit about what is exposed and b) wrap with http basic auth.
	serveStatic("/js/golongpoll-client.js", *staticDir, "golongpoll-client.js", "text/javascript", users)
	serveStatic("/js/main.js", *staticDir, "main.js", "text/javascript", users)
	serveStatic("/css/main.css", *staticDir, "main.css", "text/css", users)
	serveStatic("/favicon.svg", *staticDir, "favicon.svg", "image/svg+xml", users)
	serveStatic("/logout-favicon.svg", *staticDir, "logout-favicon.svg", "image/svg+xml", users)

	log.Println("Serving on:", *serveAddr, "staticDir at:", *staticDir, "saving chats to:", *persistFilename)
	log.Println("Listing Users...")
	for _, user := range users {
		log.Println(user.Username)
	}

	http.ListenAndServe(*serveAddr, nil)
}

func parseAccounts(input string) []User {
	parts := strings.Split(input, ",")
	if len(parts) == 0 {
		return nil
	}
	users := make([]User, 0)
	for _, part := range parts {
		userAndPassword := strings.Split(part, ":")
		if len(userAndPassword) != 2 || len(userAndPassword[0]) == 0 || len(userAndPassword[1]) == 0 {
			fmt.Printf("Invalid -accounts value: %q, must be comma separated accounts of the form user:password.", part)
			return nil
		}
		users = append(users, User{Username: userAndPassword[0], Password: userAndPassword[1]})
	}
	return users
}

func serveStatic(url string, staticDir string, staticFilename string, contentType string, users []User) {
	http.HandleFunc(url, requireBasicAuth(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			http.ServeFile(w, r, filepath.Join(staticDir, staticFilename))
		}, users))
}

// Modified flavor of function from: https://stackoverflow.com/a/39591234
// Updated to take a list of acceptable logins instead of a single user/password.
func requireBasicAuth(handler http.HandlerFunc, accounts []User) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		realm := "Please enter your username and password for this site"
		if !ok || !loginOkay(user, pass, accounts) {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			w.WriteHeader(401)
			w.Write([]byte("\n"))
			// TODO: stylign here--inline instead of fetching file (inline faviconicon data too?)
			fmt.Fprintf(w, `
			<html>
			<head>
				<title>Unauthorized</title>
				<meta name="viewport" content="width=device-width, initial-scale=1.0">
			</head>
			<body>
				<h1>Unauthorized.</h1>
			</body>
			</html>`)
			if ok {
				log.Println("LOGIN FAIL -", r.URL, "- username:", user, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
			} else {
				log.Println("LOGIN REQUIRED -", r.URL, "- IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
			}
			return
		}
		handler(w, r)
	}
}

func loginOkay(username string, password string, accounts []User) bool {
	for _, account := range accounts {
		if subtle.ConstantTimeCompare([]byte(account.Username), []byte(username)) == 1 && subtle.ConstantTimeCompare([]byte(account.Password), []byte(password)) == 1 {
			return true
		}
	}
	return false
}

func logoutPage(w http.ResponseWriter, r *http.Request) {
	// This should force browsers to forget/re-ask for creds.
	w.WriteHeader(401)
	user, _, _ := r.BasicAuth()
	log.Println("Logout -", r.URL, "- username:", user, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
	fmt.Fprintf(w, `
	<html>
	<head>
		<title>BasicChat - Logged Out</title>
		<link rel="icon" type="image/svg+xml" href="/logout-favicon.svg">
		<link rel="stylesheet" href="/css/main.css">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
	</head>
	<body>
		<p>Logged out as %s.</p>
		<p><a href="/">Home</a></p>
	</body>
	</html>
	`, user)
}

// TODO: list rooms (categories) by latest activity first, have last msg and time.
func indexPage(w http.ResponseWriter, r *http.Request) {
	// Wrapped by requireBasicAuth which will enforce that a real user+password was provided.
	username, _, ok := r.BasicAuth()
	if !ok || len(username) == 0 {
		// should only be called when user is present in basic auth--so this would be a server logic error
		// if this ever fails
		w.WriteHeader(500)
		w.Write([]byte("Failed to get user.\n"))
		return
	}
	log.Println("Homepage -", r.URL, "- username:", username, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
	fmt.Fprintf(w, `
	<html>
	<head>
		<title>BasicChat - Home</title>
		<link rel="icon" type="image/svg+xml" href="/favicon.svg">
		<link rel="stylesheet" href="/css/main.css">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
	</head>
	<body>
		<p>Hello, %s.</p>
		<p><a href="/chat?room=Neat+Stuff">Neat Stuff</a></p>
		<p><a href="/logout">Logout</a></p>
	</body>
	</html>
	`, username)
}

// TODO: conversation list
// TODO: input and send button--clear input on send
// TODO: make inptu a text area?
// TODO: enter to auto send
// TODO: logout link--make sure works as expected
// TODO: escape data--see micro-chat how it does it.
func chatroomPage(w http.ResponseWriter, r *http.Request) {
	// Wrapped by requireBasicAuth which will enforce that a real user+password was provided.
	username, _, ok := r.BasicAuth()
	if !ok || len(username) == 0 {
		// should only be called when user is present in basic auth--so this would be a server logic error
		// if this ever fails
		w.WriteHeader(500)
		w.Write([]byte("Failed to get user.\n"))
		return
	}
	// TODO: need to make sure room is escaped/sanitized! Use sanitized version below
	room := r.URL.Query().Get("room")
	if len(room) < 1 {
		w.WriteHeader(400)
		w.Write([]byte("Url param 'room' is missing or empty.\n"))
		log.Println("ChatRoom MISSING ROOM PARAN -", r.URL, "- username:", username, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
		return
	}
	log.Println("ChatRoom -", r.URL, "- username:", username, "room:", room, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
	fmt.Fprintf(w, `
	<html>
	<head>
		<title>BasicChat - %s</title>
		<link rel="icon" type="image/svg+xml" href="/favicon.svg">
		<link rel="stylesheet" href="/css/main.css">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
	</head>
	<body>
		<p>Hi, %s!.  Welcome to chatroom: %s.</p>
		<p><a href="/">Home</a></p>
		<p><a href="/logout">Logout</a></p>
		</body>
	</html>
	`, room, username, room)
}
