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
    // replace more than two consecutive newlines
    sanitizedBody = sanitizedBody.replace(/\n\s*\n\s*\n/g, '\n\n');
    // turn newlines into <br>
    sanitizedBody = sanitizedBody.replace(/(?:\r\n|\r|\n)/g, '<br>');

    // preserve spaces and turn tabs into spaces:
    sanitizedBody = sanitizedBody.replace(/\t/g, '&nbsp;&nbsp;&nbsp;&nbsp;');
    sanitizedBody = sanitizedBody.replace(/\s/g, '&nbsp;');

    // TODO: add span around user mentions

    return sanitizedBody;
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
