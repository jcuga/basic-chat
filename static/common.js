function sanitize(string) {
    const map = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#x27;',
        "/": '&#x2F;',
        '`': '&grave;',
    };
    const reg = /[&<>"'/]/ig;
    return string.replace(reg, (match)=>(map[match]));
}

function formatChatBody(msg) {
    // escape html chars
    var sanitizedBody = sanitize(msg);

    // special cases:
    if (sanitizedBody.startsWith("code:") || sanitizedBody.startsWith("Code:")) {
        return "<div class=\"chat-msg-code\">" + preserveSpaces(sanitizedBody.substring(5).trim()) + "<div>";
    }

    if (sanitizedBody.startsWith("link:") || sanitizedBody.startsWith("Link:")) {
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

    // TODO: add span around user mentions
    return preserveSpaces(sanitizedBody);
}

function preserveSpaces(input) {
    // replace more than two consecutive newlines
    input = input.replace(/\n\s*\n\s*\n/g, '\n\n');
    // turn newlines into <br>
    input = input.replace(/(?:\r\n|\r|\n)/g, '<br>');
    input = input.replace(/\t/g, '&nbsp;&nbsp;&nbsp;&nbsp;');
    input = input.replace(/\s/g, '&nbsp;');
    return input;
}

// Show timestamp as time if within last 24 hours, otherwise datetime
function chatTimestamp(timestamp) {
    var date = new Date(timestamp);
    var deltaHours = Math.floor((Date.now() - date) / (1000*60*60))
    if (deltaHours < 24) {
        return date.toLocaleTimeString();
    } else {
        return date.toLocaleString();
    }
}

function getChatMsgHtml(timestamp, sender, msg, currentUser) {
    var msgSenderClass = currentUser.toLowerCase() == sender.toLowerCase() ? "chat-from-me" : "chat-from-other";
    return "<div class=\"chat-msg\"><span class=\"chat-timestamp\">" + (chatTimestamp(timestamp)) +
            " </span><span class=\"chat-username " + msgSenderClass + "\">" + sanitize(sender) + "</span>" +
            " <span class=\"chat-body\">" + formatChatBody(msg) + "</span>" +
            "</div>"
}
