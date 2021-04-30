package main

import (
	"crypto/subtle"
	"flag"
	"fmt"
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
	chatHistorySize := flag.Uint("-historySize", 2500, "Max number of chats in history.")

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
		}, users, "Please enter your username and password for this site"))

	manager, err := golongpoll.StartLongpoll(golongpoll.Options{
		// hang on to 2500 most recent chats:
		MaxEventBufferSize: int(*chatHistorySize),
		AddOn:              filePersistor,
	})
	if err != nil {
		log.Fatalf("Failed to create chat longpoll manager: %q\n", err)
	}

	http.HandleFunc("/", requireBasicAuth(indexPage, users, "Please enter your username and password for this site"))
	http.HandleFunc("/send", requireBasicAuth(manager.PublishHandler, users, "Please enter your username and password for this site"))
	http.HandleFunc("/chats", requireBasicAuth(manager.SubscriptionHandler, users, "Please enter your username and password for this site"))

	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		// This should force browsers to forget/re-ask for creds.
		w.WriteHeader(401)
		w.Write([]byte("Logged out.\n"))
	})

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
func requireBasicAuth(handler http.HandlerFunc, accounts []User, realm string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		user, pass, ok := r.BasicAuth()

		if !ok || !loginOkay(user, pass, accounts) {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorized.\n"))
			if ok {
				log.Println("LOGIN FAIL - username:", user, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
			} else {
				log.Println("LOGIN REQUIRED - IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
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
func indexPage(w http.ResponseWriter, r *http.Request) {
	// Wrapped by requireBasicAuth which will enforce that a real user+password was provided.
	username, _, ok := r.BasicAuth()
	if !ok || len(username) == 0 {
		// should only be called when user is present in basic auth--so this would be a server logic error
		// if this ever fails
		w.WriteHeader(500)
		w.Write([]byte("Failed to get user.\n"))
	}

	fmt.Fprintf(w, `
	Hi, %s!
	`, username)
}
