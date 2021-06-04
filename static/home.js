var homeHeader = document.getElementById("greeting");
homeHeader.innerHTML = "Hello, " + sanitize(currentUsername);

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
    return "<div class=\"active-room-item\"><div><span class=\"room-timestamp\">" + timeAgoTimestamp(lastChat["timestamp"]) + "</span><a href=\"./chat?room=" + encodeURIComponent(lastChat.category) + "\">" + sanitize(room) + "</a></div>" +
     "<div>" +
     " </span><span class=\"chat-username " + msgSenderClass + "\">" + sanitize(chatMsg.username) + "</span>" +
     " <div class=\"chat-body truncate\">" + formatChatBody(chatMsg.msg) + "</div>" +
     "</div></div>";
}

window.onload = function() {
    loadRooms();
    loadUsers();

    var createRoomInput = document.getElementById("create-room-room");
    var createRoomSubmit = document.getElementById("create-room-submit");
    createRoomInput.value = '';
    createRoomInput.addEventListener("keydown", function(event) {
        if (event.key === 'Enter' && !event.shiftKey) {
        // Cancel the default action, if needed
        event.preventDefault();
        // Trigger the button element with a click
        createRoomSubmit.click();
        }
    });

    createRoomInput.addEventListener("keyup", function(event) {
        createRoomSubmit.disabled = createRoomInput.value.length == 0;
    });

};
