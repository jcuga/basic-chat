package main

import (
	"crypto/subtle"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/jcuga/golongpoll"
)

type User struct {
	Username string
	Password string
}

func main() {
	serveAddr := flag.String("serve", "127.0.0.1:8080", "Address to serve HTTP on.")
	staticClientJs := flag.String("clientJs", "./static/golongpoll-client.js", "where the static/golongpoll-client.js is located relative to where this binary runs.")
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

	// serve longpoll js client code
	http.HandleFunc("/js/golongpoll-client.js", requireBasicAuth(
		func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, *staticClientJs)
		}, users))

	manager, err := golongpoll.StartLongpoll(golongpoll.Options{
		// hang on to 2500 most recent chats:
		MaxEventBufferSize: int(*chatHistorySize),
		AddOn:              filePersistor,
	})
	if err != nil {
		log.Fatalf("Failed to create chat longpoll manager: %q\n", err)
	}

	http.HandleFunc("/", requireBasicAuth(indexPage, users))
	http.HandleFunc("/chat", requireBasicAuth(chatroomPage, users))
	http.HandleFunc("/send", requireBasicAuth(manager.PublishHandler, users))
	http.HandleFunc("/listen", requireBasicAuth(manager.SubscriptionHandler, users))
	http.HandleFunc("/logout", logoutPage)

	http.HandleFunc("/favicon.svg", requireBasicAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		fmt.Fprintf(w,
			`<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 100 100"><rect width="100" height="100" rx="20" fill="#7d6ee7"></rect><path fill="#fff" d="M30.10 38.10L30.10 38.10Q31.10 38.10 32.10 38.50Q33.10 38.90 34.30 39.30L34.30 39.30L44.90 44.70L44.30 25.90Q45.30 25.30 46.90 24.70Q48.50 24.10 50.50 24.10L50.50 24.10Q53.10 24.10 54.80 25.30Q56.50 26.50 56.50 29.30L56.50 29.30Q56.50 30.70 56.10 35.70Q55.70 40.70 55.50 45.10L55.50 45.10L70.90 36.30Q72.70 37.10 74.30 39Q75.90 40.90 75.90 43.30L75.90 43.30Q75.90 45.70 74.50 46.90Q73.10 48.10 70.90 49.10L70.90 49.10L59.10 54.70L71.70 69.70Q71.30 72.30 69.20 74.10Q67.10 75.90 64.70 75.90L64.70 75.90Q62.50 75.90 60.70 74.20Q58.90 72.50 57.50 70.70L57.50 70.70L50.30 60.70L40.10 75.70Q39.10 75.90 37.50 75.90L37.50 75.90Q33.50 75.90 31.70 74.20Q29.90 72.50 29.90 70.10L29.90 70.10Q29.90 67.10 32.30 64.70L32.30 64.70L41.50 54.30L24.70 48.70Q24.10 46.70 24.10 44.50L24.10 44.50Q24.10 41.70 25.70 39.90Q27.30 38.10 30.10 38.10Z"></path></svg>`,
		)
	}, users))

	log.Println("Serving on:", *serveAddr, "clientJs at:", *staticClientJs, "saving chats to:", *persistFilename)
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

// Modified flavor of function from: https://stackoverflow.com/a/39591234
// Updated to take a list of acceptable logins instead of a single user/password.
func requireBasicAuth(handler http.HandlerFunc, accounts []User) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		user, pass, ok := r.BasicAuth()
		realm := "Please enter your username and password for this site"
		if !ok || !loginOkay(user, pass, accounts) {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorized.\n"))
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
	generatePageWithContent(w, `Logged out.<a href="/">Home</a>`)
	user, _, _ := r.BasicAuth()
	log.Println("Logout -", r.URL, "- username:", user, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
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
	generatePageWithContent(w, fmt.Sprintf(`
	<a href="/logout">Logout</a>
	<br/>
	Hi, %s!`, username))
}

// TODO: conversation list
// TODO: input and send button--clear input on send
// TODO: make inptu a text area?
// TODO: enter to auto send
// TODO: logout link--make sure works as expected
// TODO: escape data--see micro-chat how it does it.
// TODO: styling--make own vs others diff color/style/class
// TODO: UI click to change (increase... and decrease? display N hours or N day ) sinceTime?
// TODO: and/or have UI actually hide/unhide some?
// TODO: light vs dark mode -- cookies or url param?
// TODO: screenshot in readme because it looks pretty
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

	room := r.URL.Query().Get("room")
	if len(room) < 1 {
		w.WriteHeader(400)
		w.Write([]byte("Url param 'room' is missing or empty.\n"))
		log.Println("ChatRoom MISSING ROOM PARAN -", r.URL, "- username:", username, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
		return
	}

	log.Println("ChatRoom -", r.URL, "- username:", username, "room:", room, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))

	generatePageWithContent(w, fmt.Sprintf(`
	<a href="/">Home</a>
	<a href="/logout">Logout</a>
	<br/>
	Hi, %s!.  Welcome to chatroom: %s
	`, username, room))
}

func generatePageWithContent(w io.Writer, content string) {
	fmt.Fprintf(w, `
	<html>
	<head>
		<title>Chat</title>
		<link rel="icon" type="image/svg+xml" href="/favicon.svg">
		<style>
			body {
				background-color: white;
				color: black;
			    font-family: Arial, Helvetica, sans-serif;
			}
		</style>
	</head>
	<body>
%s
	</body>
	</html>
	`, content)
}
