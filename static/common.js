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

    // TODO: add span around user mentions

    return sanitizedBody;
}