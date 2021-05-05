package main

import (
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcuga/golongpoll"
	"github.com/microcosm-cc/bluemonday"
	cmap "github.com/orcaman/concurrent-map"
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

	lastEventPerCategoryAddOn := LastEventPerCategoryAddOn{
		FilePersistor:        *filePersistor,
		LastEventPerCategory: cmap.New(),
	}
	manager, err := golongpoll.StartLongpoll(golongpoll.Options{
		// hang on to N most recent chats:
		MaxEventBufferSize: int(*chatHistorySize),
		AddOn:              &lastEventPerCategoryAddOn,
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

	// chat specific api:
	http.HandleFunc("/last-chats", requireBasicAuth(getLastChatPerCategory(&lastEventPerCategoryAddOn), users))
	http.HandleFunc("/create-room", requireBasicAuth(getCreateRoom(manager), users))

	// Serve static files--doing files explicitly instead of http.FileServer so
	// we can a) be explicit about what is exposed and b) wrap with http basic auth.
	serveStatic("/js/client.js", *staticDir, "golongpoll-client.js", "text/javascript", users)
	serveStatic("/js/home.js", *staticDir, "home.js", "text/javascript", users)
	serveStatic("/js/chatroom.js", *staticDir, "chatroom.js", "text/javascript", users)
	serveStatic("/js/common.js", *staticDir, "common.js", "text/javascript", users)
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
			fmt.Fprintf(w, `
			<html>
			<head>
				<title>Unauthorized</title>
				<meta name="viewport" content="width=device-width, initial-scale=1.0">
			</head>
			<body>
				<h1>Unauthorized.</h1>
				<a href="/">Login</a>
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
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(account.Username)), []byte(strings.ToLower(username))) == 1 &&
			subtle.ConstantTimeCompare([]byte(account.Password), []byte(password)) == 1 {
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

func sanitizeInput(input string) string {
	return bluemonday.UGCPolicy().Sanitize(input)
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

		<form action="/create-room" method="post">
			<label for="create-room-room">Create Chat Room:</label>
			<input type="text" id="create-room-room" name="room"><br>
			<input type="submit" value="Submit">
		</form>

		<script src="/js/client.js"></script>
		<script src="/js/common.js"></script>
		<script src="/js/home.js"></script>
	</body>
	</html>
	`, username)
}

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
	sanitizedRoom := sanitizeInput(room)
	log.Println("ChatRoom -", r.URL, "- username:", username, "room:", room, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
	fmt.Fprintf(w, `
	<html>
	<head>
		<title>%s</title>
		<link rel="icon" type="image/svg+xml" href="/favicon.svg">
		<link rel="stylesheet" href="/css/main.css">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
	</head>
	<body>
		<p>Hi, %s!.  Chatroom: %s.</p>
		<p><a href="/">Home</a></p>
		<p><a href="/logout">Logout</a></p>
		<div id="chat-conv"></div>
		<textarea id="chat-input"></textarea>
		<button type="button" id="chat-send">Send</button>

		<script>
			var chatroomCategory="%s";
			var chatroomUsername="%s";
		</script>

		<script src="/js/client.js"></script>
		<script src="/js/common.js"></script>
		<script src="/js/chatroom.js"></script>
		</body>
	</html>
	`, sanitizedRoom, username, sanitizedRoom, sanitizedRoom, username)
}

// Wraps golongpoll.FilePersistorAddOn with logic to keep track of the
// most recent chat message per category (ie chatroom).
type LastEventPerCategoryAddOn struct {
	FilePersistor        golongpoll.FilePersistorAddOn
	LastEventPerCategory cmap.ConcurrentMap
}

func (a *LastEventPerCategoryAddOn) OnPublish(event *golongpoll.Event) {
	a.FilePersistor.OnPublish(event)
	a.LastEventPerCategory.Set(event.Category, event)
}

func (a *LastEventPerCategoryAddOn) OnShutdown() {
	a.FilePersistor.OnShutdown()
}

func (a *LastEventPerCategoryAddOn) OnLongpollStart() <-chan *golongpoll.Event {
	fileChan := a.FilePersistor.OnLongpollStart()
	ch := make(chan *golongpoll.Event, 100)
	go a.getOnStartInputEvents(fileChan, ch)
	return ch
}

func (a *LastEventPerCategoryAddOn) getOnStartInputEvents(fileChan <-chan *golongpoll.Event, outChan chan *golongpoll.Event) {
	// Pass the fileChan events along while updating the a.LastEventPerCategory map.
	// Note that events come in in chronological order (oldest first).
	for {
		event, ok := <-fileChan
		if ok {
			a.LastEventPerCategory.Set(event.Category, event)
			outChan <- event
		} else {
			// channel closed, we're done. Close our out channel.
			close(outChan)
			break
		}
	}
}

func getLastChatPerCategory(a *LastEventPerCategoryAddOn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		copiedMap := a.LastEventPerCategory.Items()
		if jsonData, err := json.Marshal(copiedMap); err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, string(jsonData))
		} else {
			log.Println("ERROR: failed to marshal lastChatPerCategory map.", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func getCreateRoom(lpManager *golongpoll.LongpollManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			log.Println("WARN - createRoom invalid methid:", r.Method)
			fmt.Fprintf(w, "Method Not Allowed")
			return
		}

		// Wrapped by requireBasicAuth which will enforce that a real user+password was provided.
		username, _, ok := r.BasicAuth()
		if !ok || len(username) == 0 {
			// should only be called when user is present in basic auth--so this would be a server logic error
			// if this ever fails
			w.WriteHeader(500)
			w.Write([]byte("Failed to get user.\n"))
			return
		}

		r.ParseForm()
		room := r.Form.Get("room")
		if len(room) < 1 {
			w.WriteHeader(400)
			w.Write([]byte("Url param 'room' is missing or empty.\n"))
			log.Println("CreateChatRoom MISSING ROOM PARAN -", r.URL, "- username:", username, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
			return
		}

		// Generate create-room message. Aside from being informative, this ensures we have
		// a longpoll category and get an item for this room in getLastChatPerCategory()
		lpManager.Publish(room, fmt.Sprintf("Chatroom: \"%s\" created by %s.", room, username))

		// Redirect to chatroom
		newUrl := "/chat?room=" + url.QueryEscape(room)
		http.Redirect(w, r, newUrl, http.StatusSeeOther)
	}
}
