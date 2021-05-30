var homeHeader = document.getElementById("home-header");
homeHeader.innerHTML = "Hello, " + sanitize(currentUsername);

function loadUsers() {
    var xhttp = new XMLHttpRequest();
    xhttp.onreadystatechange = function() {
        if (this.readyState == 4 && this.status == 200) {
            var resp = JSON.parse(this.responseText);
            var userActiveList = Object.entries(resp);
            var sortedUsers = userActiveList.sort(function(a, b){return b[1] - a[1]});
            usersDiv = document.getElementById("recent-users");
            usersDiv.innerHTML = "";
            for (var i = 0; i < sortedUsers.length; ++i) {
                usersDiv.insertAdjacentHTML('beforeend', getUserHtml(sortedUsers[i][0], sortedUsers[i][1]));
            }
            setTimeout(loadUsers, 30000);
        }
    };
    xhttp.open("GET", "/users", true);
    xhttp.send();
}

function getUserHtml(username, timestamp) {
    return "<div class=\"active-user-item\">" + username + ": " + timeAgoTimestamp(timestamp) + "</div>";
}

function loadRooms() {
    var xhttp = new XMLHttpRequest();
    xhttp.onreadystatechange = function() {
        if (this.readyState == 4 && this.status == 200) {
            var resp = JSON.parse(this.responseText);
            var chatsList = Object.entries(resp);
            var sortedChats = chatsList.sort(function(a, b){return b[1]["timestamp"] - a[1]["timestamp"]});
            roomsDiv = document.getElementById("recent-rooms");
            roomsDiv.innerHTML = "";
            for (var i = 0; i < sortedChats.length; ++i) {
                roomsDiv.insertAdjacentHTML('beforeend', getRoomHtml(sortedChats[i][0], sortedChats[i][1]));
            }
            setTimeout(loadRooms, 30000);
        }
    };
    xhttp.open("GET", "/last-chats", true);
    xhttp.send();
}

function getRoomHtml(room, lastChat) {
    var chatMsg = lastChat["data"];
    var msgSenderClass = currentUsername.toLowerCase() == chatMsg.username.toLowerCase() ? "chat-from-me" : "chat-from-other";
    return "<div class=\"active-room-item\"><div><a href=\"./chat?room=" + encodeURIComponent(lastChat.category) + "\">" + sanitize(room) + "</a> <span class=\"room-timestamp\">" + timeAgoTimestamp(lastChat["timestamp"]) + "</span></div>" +
     "<div>" +
     " </span><span class=\"chat-username " + msgSenderClass + "\">" + sanitize(chatMsg.username) + "</span>" +
     " <span class=\"chat-body\">" + formatChatBody(chatMsg.msg) + "</span>" +
     "</div></div>";
}

window.onload = function() {
    loadRooms();
    loadUsers();
};