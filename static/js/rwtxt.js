// websockets 
var socket;
const socketMessageListener = (event) => {
    // console.log(event);
    CY.serverResponse(event.data);
};
const socketOpenListener = (event) => {
    // console.log('Connected');
    document.getElementById("notsaved").style.display = 'none';
    document.getElementById("connectedicon").style.display = 'inline-block';
    setTimeout(function () {
        document.getElementById("connectedicon").style.display = 'none';
    }, 1000);
};
const socketCloseListener = (event) => {
    if (socket) {
        console.error('Disconnected.');
        document.getElementById("notsaved").style.display = 'inline-block';
    }
    var url = window.origin.replace("http", "ws") + '/ws';
    socket = new WebSocket(url);
    socket.addEventListener('open', socketOpenListener);
    socket.addEventListener('message', socketMessageListener);
    socket.addEventListener('close', socketCloseListener);
    // console.log('opening socket to ' + url)
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
    // console.log('edited');
    var markdown = document.getElementById("editable").value.replaceAll("<br>", "\n");
    var slug = slugify(markdown);
    socket.send(JSON.stringify({
        "id": window.rwtxt.file_id,
        "slug": slugify(markdown),
        "data": markdown,
        "domain": window.rwtxt.domain,
        "domain_key": window.rwtxt.domain_key
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
        // console.log(newwindowname);
        if (newwindowname != undefined && newwindowname.length > 0 && "/" + newwindowname != window.location
            .pathname) {
            history.replaceState({}, newwindowname, newwindowname);
            document.title = newwindowname + " | " + window.rwtxt.domain;
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
    // console.log('loading editor');
    showMessage();
};

document.getElementById("editable").addEventListener('input', CY.debounce(CY.contentEdited, 200));

// allow tabs
document.getElementById("editable").onkeydown = function(e) {
    if(e.keyCode==9 || e.which==9 || e.key == "Tab"){
        e.preventDefault();
        var s = this.selectionStart;
        this.value = this.value.substring(0,this.selectionStart) + "\t" + this.value.substring(this.selectionEnd);
        this.selectionEnd = s+1; 
    }
}

editlink = document.getElementById("editlink")
if (editlink != null) {
    editlink.addEventListener("click", CY.loadEditor);
}


document.getElementById("editable").addEventListener('focusin', function (e) {
    // console.log('focusin!')
    editor = document.getElementById("editable");
    // console.log('[' + editor.value.trim() + ']');
    if (editor.value.trim() == window.rwtxt.intro_text) {
        editor.value = " ";
    }
})


var autoExpand = function (field) {
    // Get the computed styles for the element
    var computed = window.getComputedStyle(field);
    // Calculate the height
    var height = parseInt(computed.getPropertyValue('border-top-width'), 10) +
        parseInt(computed.getPropertyValue('padding-top'), 10) +
        field.scrollHeight +
        parseInt(computed.getPropertyValue('padding-bottom'), 10) +
        parseInt(computed.getPropertyValue('border-bottom-width'), 10);
    if (field.style.height != height + 'px') {
        // Reset field height
        field.style.height = 'inherit';
        field.style.height = height + 'px';    
    }
};

document.getElementById("editable").addEventListener('input', function (event) {
    autoExpand(event.target);
}, false);


// if editing, go to edit page
if (getParameterByName("edit") != null) {
    CY.loadEditor();
    document.getElementById("editable").focus();
    history.pushState({}, window.location.pathname, window.location.pathname);
}

function showMessage() {
    var x = document.getElementById("snackbar");
    if (x != null) {
        x.className = "show";
        setTimeout(function(){ x.className = x.className.replace("show", ""); }, 3000);
    }
}


function onUploadFinished(file) {
    // // console.log("upload finished");
    // // console.log(file);
    this.removeFile(file);
    var cursorPos = document.getElementById("editable").selectionStart;
    var cursorEnd = document.getElementById("editable").selectionEnd;
    var v = document.getElementById("editable").value;
    var textBefore = v.substring(0,  cursorPos);
    var textAfter  = v.substring(cursorPos, v.length);
    var message = 'uploaded file';
    if (cursorEnd > cursorPos) {
        message = v.substring(cursorPos, cursorEnd);
        textAfter = v.substring(cursorEnd, v.length);
    }
    var prefix = '';
    if (file.type.startsWith("image")) {
        prefix = '!';
    }
    var extraText = prefix+'['+file.xhr.getResponseHeader("Location").split('filename=')[1]+'](' +
        file.xhr.getResponseHeader("Location") +
        ')';

    var newLine = "\n"
    document.getElementById("editable").value = (
        textBefore +
        extraText + 
        newLine +
        textAfter
    );

    console.log("SELECT LINK")
    // Select the newly-inserted link
    document.getElementById("editable").selectionStart = cursorPos + extraText.length + newLine.length;
    document.getElementById("editable").selectionEnd = cursorPos + extraText.length + newLine.length;
   // expand textarea
    autoExpand(document.getElementById("editable"));
   // trigger a save
   CY.contentEdited();
}

// if editing, keep focus always on the editable
window.onclick= function (event) {
    if (document.getElementById("editable").style.display != "none") {
       if (event.target.nodeName == "HTML") {
            document.getElementById("editable").focus();
       } 
    }
}

if (window.rwtxt.domain_key != "") {
    Dropzone.options.dropzoneForm = {
        clickable: false,
        maxFilesize:   10  , 
        init: function initDropzone() {
            this.on("complete", onUploadFinished);
        }
    };    
}

if (window.rwtxt.editonly == "yes") {
    socketCloseListener();
    showMessage();
}