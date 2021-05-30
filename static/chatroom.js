var roomHeader = document.getElementById("room-header");
roomHeader.innerHTML = "Hello, " + sanitize(currentUsername) + ". Room: " + sanitize(chatroomCategory);

var client = golongpoll.newClient({
    subscribeUrl: "./events",
    category: chatroomCategory,
    publishUrl: "./publish",
    // Get all events ever for given chatroom, the UI will show only the last N
    // and page backwards/show-more.
    sinceTime: 1,
    loggingEnabled: false,
    onEvent: function (event) {
        document.getElementById("chat-conv").insertAdjacentHTML('beforeend', 
            getChatMsgHtml(event.timestamp, event.data["username"], event.data["msg"], currentUsername));
    },
});

var sendButton = document.getElementById("chat-send");
var chatInput = document.getElementById("chat-input");

chatInput.focus();
chatInput.select();

sendButton.onclick = function(event) {
    var message = chatInput.value;
    if (message.length == 0) {
        // TODO: add error message in DOM instead of using alert
        // TODO: or disable/enable based on content
        alert("message cannot be empty");
        return;
    }
    sendButton.disabled = true;
    client.publish(chatroomCategory, {username: currentUsername, msg: message},
        function () {
            chatInput.value = '';
            chatInput.focus();
            chatInput.select();
            sendButton.disabled = false;
        },
        function(status, resp) {
            // TODO: add error message in DOM instead of alert?
            alert("publish post request failed. status: " + status + ", resp: " + resp);
            chatInput.focus();
            chatInput.select();
            sendButton.disabled = false;
        }
    );
};

chatInput.addEventListener("keydown", function(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
      // Cancel the default action, if needed
      event.preventDefault();
      // Trigger the button element with a click
      sendButton.click();
    }
  });
