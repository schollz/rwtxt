// websockets 
var socket;
const socketMessageListener = (event) => {
    console.log(event);
    CY.serverResponse(event.data);
};
const socketOpenListener = (event) => {
    console.log('Connected');
};
const socketCloseListener = (event) => {
    if (socket) {
        console.error('Disconnected.');
    }
    var url = window.origin.replace("http", "ws") + '/ws';
    socket = new WebSocket(url);
    socket.addEventListener('open', socketOpenListener);
    socket.addEventListener('message', socketMessageListener);
    socket.addEventListener('close', socketCloseListener);
};

// get URL query parameters 
function getParameterByName(name, url) {
    if (!url) url = window.location.href;
    name = name.replace(/[\[\]]/g, '\\$&');
    var regex = new RegExp('[?&]' + name + '(=([^&#]*)|&|#|$)'),
        results = regex.exec(url);
    if (!results) return null;
    if (!results[2]) return '';
    return decodeURIComponent(results[2].replace(/\+/g, ' '));
}


// slugify the current text
function slugify(text) {
    var lines = text.split('\n');
    for (var i = 0; i < lines.length; i++) {
        var slug = lines[i].toString().toLowerCase()
            .replace(/\s+/g, '-') // Replace spaces with -
            .replace(/[^\w\-]+/g, '') // Remove all non-word chars
            .replace(/\-\-+/g, '-') // Replace multiple - with single -
            .replace(/^-+/, '') // Trim - from start of text
            .replace(/-+$/, ''); // Trim - from end of text
        if (slug.length > 1) {
            return slug;
        }
    }
    return "";
}

// replace all function
String.prototype.replaceAll = function (search, replacement) {
    var target = this;
    return target.replace(new RegExp(search, 'g'), replacement);
};

var div = document.getElementById('editable');
setTimeout(function () {
    div.focus();
}, 0);

var CY = {};
CY.debounce = function (func, wait, immediate) {
    var timeout;
    return function () {
        var context = this,
            args = arguments;
        var later = function () {
            timeout = null;
            if (!immediate) {
                func.apply(context, args);
            }
        };
        var callNow = immediate && !timeout;
        clearTimeout(timeout);
        timeout = setTimeout(later, wait || 200);
        if (callNow) {
            func.apply(context, args);
        }
    };
};

CY.contentEdited = function () {
    console.log('edited');
    var markdown = document.getElementById("editable").value.replaceAll("<br>", "\n");
    var slug = slugify(markdown);
    socket.send(JSON.stringify({
        "id": window.cowyo2.file_id,
        "slug": slugify(markdown),
        "data": markdown,
        "domain": window.cowyo2.domain,
        "domain_key": window.cowyo2.domain_key
    }));
};

CY.serverResponse = function (jsonString) {
    var data = JSON.parse(jsonString);
    if (data.message == "unique_slug") {
        var newwindowname = ""
        if (data.success) {
            newwindowname = data.slug;
        } else {
            newwindowname = data.id;
        }
        console.log(newwindowname);
        if (newwindowname != undefined && newwindowname.length > 0 && "/" + newwindowname != window.location
            .pathname) {
            history.pushState({}, newwindowname, newwindowname);
            document.title = newwindowname;
        }
        document.getElementById("saved").style.display = 'inline-block';
        setTimeout(function () {
            document.getElementById("saved").style.display = 'none';
        }, 1000);
    } else if (data.message == "not saving") {
        document.getElementById("notsaved").style.display = 'inline-block';
        setTimeout(function () {
            document.getElementById("notsaved").style.display = 'none';
        }, 1000);
    }
}

CY.editClick = function (e) {
    e.preventDefault();
    CY.loadEditor();
}

CY.loadEditor = function () {
    socketCloseListener();
    d = document.getElementById("rendered")
    d.innerHTML = "";
    editor = document.getElementById("editable")
    //  editor.contentEditable = "true";
    editor.style.display = 'inline-block'; // needed to add brs at end
    editor.focus();
    autoExpand(document.getElementById("editable"));
    console.log('loading editor');
};

document.getElementById("editable").addEventListener('input', CY.debounce(CY.contentEdited, 300));

editlink = document.getElementById("editlink")
if (editlink != null) {
    editlink.addEventListener("click", CY.loadEditor);
}


document.getElementById("editable").addEventListener('focusin', function (e) {
    console.log('focusin!')
    editor = document.getElementById("editable");
    console.log('[' + editor.value.trim() + ']');
    if (editor.value.trim() == window.cowyo2.intro_text) {
        editor.value = " ";
    }
})


var autoExpand = function (field) {
    // Reset field height
    field.style.height = 'inherit';
    // Get the computed styles for the element
    var computed = window.getComputedStyle(field);
    // Calculate the height
    var height = parseInt(computed.getPropertyValue('border-top-width'), 10) +
        parseInt(computed.getPropertyValue('padding-top'), 10) +
        field.scrollHeight +
        parseInt(computed.getPropertyValue('padding-bottom'), 10) +
        parseInt(computed.getPropertyValue('border-bottom-width'), 10);

    field.style.height = height + 'px';
};

document.getElementById("editable").addEventListener('input', function (event) {
    if (event.target.tagName.toLowerCase() !== 'textarea') return;
    autoExpand(event.target);
}, false);


// if editing, go to edit page
if (getParameterByName("edit") != null) {
    CY.loadEditor();
    document.getElementById("editable").focus();
    history.pushState({}, window.location.pathname, window.location.pathname);
}