package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcuga/golongpoll"
	"github.com/microcosm-cc/bluemonday"
	cmap "github.com/orcaman/concurrent-map"
)

const (
	SystemUser = "BasicChat Bot"
)

type User struct {
	Username string
	Password string
}

type ChatMsg struct {
	Username string `json:"username"`
	Message  string `json:"msg"`
}

type UserMention struct {
	DisplayRoom      string `json:"room"`
	RoomLink         string `json:"room_link"`
	RoomNotSanitized string `json:"room_original"`
	Sender           string `json:"sender"`
	Message          string `json:"msg"`
	OriginalMessage  string `json:"original_msg"`
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

	userLastActiveMap := cmap.New()
	for _, user := range users {
		userLastActiveMap.Set(strings.ToLower(user.Username), int64(0))
	}

	// web app pages
	http.HandleFunc("/", requireBasicAuth(indexPage, users, userLastActiveMap))
	http.HandleFunc("/chat", requireBasicAuth(chatroomPage, users, userLastActiveMap))

	// longpoll pub-sub api:
	http.HandleFunc("/publish", requireBasicAuth(wrapPublishHandler(manager, users), users, userLastActiveMap))
	http.HandleFunc("/events", requireBasicAuth(manager.SubscriptionHandler, users, userLastActiveMap))

	// chat specific api:
	http.HandleFunc("/last-chats", requireBasicAuth(getLastChatPerCategory(&lastEventPerCategoryAddOn), users, userLastActiveMap))
	http.HandleFunc("/create-room", requireBasicAuth(getCreateRoom(manager), users, userLastActiveMap))
	http.HandleFunc("/users", requireBasicAuth(getUsers(users, userLastActiveMap), users, userLastActiveMap))

	// Serve static files--doing files explicitly instead of http.FileServer so
	// we can a) be explicit about what is exposed and b) wrap with http basic auth.
	serveStatic("/js/client.js", *staticDir, "golongpoll-client.js", "text/javascript", users, userLastActiveMap)
	serveStatic("/js/home.js", *staticDir, "home.js", "text/javascript", users, userLastActiveMap)
	serveStatic("/js/chatroom.js", *staticDir, "chatroom.js", "text/javascript", users, userLastActiveMap)
	serveStatic("/js/common.js", *staticDir, "common.js", "text/javascript", users, userLastActiveMap)
	serveStatic("/css/main.css", *staticDir, "main.css", "text/css", users, userLastActiveMap)
	serveStatic("/favicon.svg", *staticDir, "favicon.svg", "image/svg+xml", users, userLastActiveMap)

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

func serveStatic(url string, staticDir string, staticFilename string, contentType string, users []User, userLastActiveMap cmap.ConcurrentMap) {
	http.HandleFunc(url, requireBasicAuth(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			http.ServeFile(w, r, filepath.Join(staticDir, staticFilename))
		}, users, userLastActiveMap))
}

// Modified flavor of function from: https://stackoverflow.com/a/39591234
// Updated to take a list of acceptable logins instead of a single user/password.
func requireBasicAuth(handler http.HandlerFunc, accounts []User, userLastActiveMap cmap.ConcurrentMap) http.HandlerFunc {
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
				<style>
					h1 { font-size: 5em; }
				</style>
			</head>
			<body>
				<h1>Unauthorized.</h1>
				<a href="./">Login</a>
			</body>
			</html>`)
			if ok {
				log.Println("LOGIN FAIL -", r.URL, "- username:", user, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
			} else {
				log.Println("LOGIN REQUIRED -", r.URL, "- IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))
			}
			return
		}

		// Update last active time for user--used for online/idle/offline statuses
		userLastActiveMap.Set(strings.ToLower(user), time.Now().UnixNano()/int64(time.Millisecond))

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

// ensure published data's username matches the http auth's username
// this also only allows the ChatMsg object to be published, not arbitrary data
// payload types.
func wrapPublishHandler(lpManager *golongpoll.LongpollManager, users []User) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := ioutil.ReadAll(r.Body)
		r.Body.Close() //  must close
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

		var pubData golongpoll.PublishData
		err := json.Unmarshal(bodyBytes, &pubData)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\": \"Invalid POST body json.\"}")
			log.Printf("WARN - prePublish - Invalid post body json, error: %v\n", err)
			return
		}

		username, _, ok := r.BasicAuth()
		if !ok || len(username) == 0 {
			// should only be called when user is present in basic auth--so this would be a server logic error
			// if this ever fails
			w.WriteHeader(500)
			w.Write([]byte("Failed to get user.\n"))
			return
		}

		dataMap, ok := pubData.Data.(map[string]interface{})
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\": \"Invalid publish data, unexpected data type.\"}")
			log.Printf("WARN - prePublish - Invalid publish data, must be map[string]interface{}, got %T.\n", pubData.Data)
			return
		}

		chatUsername, ok := dataMap["username"].(string)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\": \"Invalid publish data, unexpected chat username data type.\"}")
			log.Printf("WARN - prePublish - Invalid publish data, expected username as string, got: %T\n", dataMap["username"])
			return
		}

		if strings.ToLower(chatUsername) != strings.ToLower(username) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\": \"Invalid publish data, username mismatch.\"}")
			log.Printf("WARN - prePublish - Invalid publish data, username mismatch.  Logged in as: %s, but got: %s.\n", username, chatUsername)
			return
		}

		msg, ok := dataMap["msg"].(string)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\": \"Invalid publish data, unexpected chat msg data type.\"}")
			log.Printf("WARN - prePublish - Invalid publish data, expected msg as string, got: %T\n", dataMap["msg"])
			return
		}

		if len(msg) > 1024*16 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "{\"error\": \"Invalid publish data, message must be <= 16kb.\"}")
			log.Printf("WARN - prePublish - Message too long, got length: %d, must be < 16kb\n", len(msg))
			return
		}

		// Look for @username and issue a notification for any mentions
		normMsg := strings.ToLower(msg)
		for _, user := range users {
			item := "@" + strings.ToLower(user.Username)
			if strings.Contains(normMsg, item) {
				category := "_____" + item
				userMention := UserMention{
					Message:          fmt.Sprintf("%s mentioned you in room: %s", username, pubData.Category),
					RoomLink:         "./chat?room=" + url.QueryEscape(pubData.Category),
					DisplayRoom:      sanitizeInput(pubData.Category),
					RoomNotSanitized: pubData.Category,
					OriginalMessage:  msg,
					Sender:           username,
				}
				lpManager.Publish(category, userMention)
			}
		}

		lpManager.PublishHandler(w, r)
	}
}

func sanitizeInput(input string) string {
	return bluemonday.UGCPolicy().Sanitize(input)
}

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
		<link rel="icon" type="image/svg+xml" href="./favicon.svg">
		<link rel="stylesheet" href="./css/main.css">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
	</head>
	<body>
		<script>var currentUsername = "%s";</script>
		<div id="home-header"></div>

		<h2>Topics</h2>
		<div id="recent-rooms"></div>

		<h2>Create Topic</h2>
		<form action="./create-room" method="post">
			<label for="create-room-room">Create Chat Room:</label>
			<input type="text" id="create-room-room" name="room"><br>
			<input type="submit" value="Submit">
		</form>
		<h2>Users</h2>
		<div id="recent-users"></div>

		<script src="./js/client.js"></script>
		<script src="./js/common.js"></script>
		<script src="./js/home.js"></script>
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
		<link rel="icon" type="image/svg+xml" href="./favicon.svg">
		<link rel="stylesheet" href="./css/main.css">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
	</head>
	<body>
		<div id="room-header"></div>
		<p><a href="./">Home</a></p>
		<div id="chat-conv"></div>
		<textarea id="chat-input" maxlength="16384"></textarea>
		<button type="button" id="chat-send">Send</button>

		<script>
			var chatroomCategory="%s";
			var currentUsername="%s";
		</script>

		<script src="./js/client.js"></script>
		<script src="./js/common.js"></script>
		<script src="./js/chatroom.js"></script>
		</body>
	</html>
	`, sanitizedRoom, sanitizedRoom, username)
}

// Wraps golongpoll.FilePersistorAddOn with logic to keep track of the
// most recent chat message per category (ie chatroom).
type LastEventPerCategoryAddOn struct {
	FilePersistor        golongpoll.FilePersistorAddOn
	LastEventPerCategory cmap.ConcurrentMap
}

func (a *LastEventPerCategoryAddOn) OnPublish(event *golongpoll.Event) {
	a.FilePersistor.OnPublish(event)
	// Ignore the user-mention notifications
	if !strings.HasPrefix(event.Category, "_____@") {
		a.LastEventPerCategory.Set(event.Category, event)
	}
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
			// Ignore the user-mention notifications
			if !strings.HasPrefix(event.Category, "_____@") {
				a.LastEventPerCategory.Set(event.Category, event)
			}

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
		username, _, _ := r.BasicAuth()
		log.Println("getLastChatPerCategory -", r.URL, "- username:", username, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))

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

func getUsers(users []User, userLastActiveMap cmap.ConcurrentMap) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, _, _ := r.BasicAuth()
		log.Println("getUsers -", r.URL, "- username:", username, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))

		copiedMap := userLastActiveMap.Items()
		if jsonData, err := json.Marshal(copiedMap); err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, string(jsonData))
		} else {
			log.Println("ERROR: failed to marshal userLastActiveMap.", err)
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
			log.Println("ERROR - Failed to get user.")
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

		log.Println("createRoom -", r.URL, "- username:", username, "room:", room, "IP:", r.RemoteAddr, "X-FORWARDED-FOR:", r.Header.Get("X-FORWARDED-FOR"))

		chatMsg := ChatMsg{
			Username: SystemUser,
			Message:  fmt.Sprintf("Chatroom: \"%s\" created by %s.", room, username),
		}

		// Generate create-room message. Aside from being informative, this ensures we have
		// a longpoll category and get an item for this room in getLastChatPerCategory()
		lpManager.Publish(room, chatMsg)

		// Redirect to chatroom
		newUrl := "./chat?room=" + url.QueryEscape(room)
		http.Redirect(w, r, newUrl, http.StatusSeeOther)
	}
}
