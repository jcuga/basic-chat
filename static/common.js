function sanitize(string) {
    const map = {
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#x27;',
        '`': '&grave;',
    };
    const reg = /[<>"']/ig;
    return string.replace(reg, (match)=>(map[match]));
}

function formatChatBody(msg) {
    // escape html chars
    var sanitizedBody = sanitize(msg);

    // special cases:
    if (sanitizedBody.startsWith("code:") || sanitizedBody.startsWith("Code:") || sanitizedBody.startsWith("CODE:")) {
        return "<div class=\"chat-msg-code\">" + preserveSpaces(sanitizedBody.substring(5).trim()) + "<div>";
    }

    if (sanitizedBody.startsWith("link:") || sanitizedBody.startsWith("Link:") || sanitizedBody.startsWith("LINK:")) {
        var linkCandidate = sanitizedBody.substring(5).trim();
        if (!linkCandidate.includes("\n")) {
            if (linkCandidate.startsWith("www.")) {
                linkCandidate = "http://" + linkCandidate;
            }
            if (linkCandidate.toLowerCase().startsWith("http://") || linkCandidate.toLowerCase().startsWith("https://")) {
                return "<a class=\"chat-msg-link\" target=\"_blank\" rel=\"noopener noreferrer\" href=\"" + linkCandidate + "\">" + linkCandidate + "</a>";
            }
        }
    }

    // Add span around any user-mentions (@username)
    sanitizedBody = sanitizedBody.replace(/(^|\s)(@\w+)/g, '$1<span class="user-mention">$2</span>');

    // TODO: add span around user mentions
    return preserveSpaces(sanitizedBody);
}

function preserveSpaces(input) {
    // replace more than two consecutive newlines
    input = input.replace(/\n\s*\n\s*\n/g, '\n\n');
    // turn newlines into <br>
    input = input.replace(/(?:\r\n|\r|\n)/g, '<br>');
    // turn tabs into spaces, and preserve multiple spaces.
    // NOTE: don't convert every whitespace into nbsp as the non-breaking
    // aspect makes wordwrap stuff not happen.
    input = input.replace(/\t/g, '&nbsp;&nbsp;&nbsp;&nbsp;');
    input = input.replace(/\s\s/g, '&nbsp;&nbsp;');
    return input;
}

// Show timestamp as time if within last 24 hours, otherwise datetime
function chatTimestamp(timestamp) {
    var date = new Date(timestamp);
    var deltaHours = Math.floor((Date.now() - date) / (1000*60*60));
    if (deltaHours < 24) {
        return date.toLocaleTimeString();
    } else {
        return date.toLocaleString();
    }
}

function timeAgoTimestamp(timestamp) {
    // special case for zero value
    if (timestamp == 0) {
        return "Never";
    }
    var date = new Date(timestamp);
    var deltaSeconds = Math.floor((Date.now() - date) / (1000));
    if (deltaSeconds < 60) {
        return "Less than a minute ago";
    } else if (deltaSeconds < 2*60) {
        return "1 minute ago";
    } else if (deltaSeconds < 60*60) {
        return Math.floor(deltaSeconds/60) + " minutes ago";
    } else if (deltaSeconds < 2*60*60) {
        return "1 hour ago";
    } else if (deltaSeconds < 24*60*60) {
        return Math.floor(deltaSeconds/(60*60)) + " hours ago";
    } else if (deltaSeconds < 2*24*60*60) {
        return "1 day ago";
    } else {
        return Math.floor(deltaSeconds/(60*60*24)) + " days ago";
    }
}

var noteClient = golongpoll.newClient({
    subscribeUrl: "./events",
    category: "_____@" + currentUsername,
    publishUrl: "./publish",
    // Get all events ever for given user, the UI will show only the last N
    // and page backwards/show-more.
    sinceTime: 1,
    loggingEnabled: false,
    onEvent: function (event) {
        document.getElementById("notifications").insertAdjacentHTML('beforeend', 
            getNotificationHtml(event.timestamp, event.data));
    },
});


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

function getNotificationHtml(timestamp, data) {
    return "<div class=\"notification\">" +
        "<a class=\"notification-title\" href=\"" + data.room_link+ "\">" + data.msg + "</a>" +
        "<div class=\"notification-timestam\">" + timeAgoTimestamp(timestamp) +"</div>" +
        "<div class=\"notification-msg\">" + data.original_msg + "</div>" +
    "</div>";
}
