 function setEndOfContenteditable(contentEditableElement) {
     var range, selection;
     if (document.createRange) //Firefox, Chrome, Opera, Safari, IE 9+
     {
         range = document.createRange(); //Create a range (a range is a like the selection but invisible)
         range.selectNodeContents(contentEditableElement); //Select the entire contents of the element with the range
         range.collapse(false); //collapse the range to the end point. false means collapse to end rather than the start
         selection = window.getSelection(); //get the selection object (allows you to change selection)
         selection.removeAllRanges(); //remove any selections already made
         selection.addRange(range); //make the range you have just created the visible selection
     } else if (document.selection) //IE 8 and lower
     {
         range = document.body.createTextRange(); //Create a range (a range is a like the selection but invisible)
         range.moveToElementText(contentEditableElement); //Select the entire contents of the element with the range
         range.collapse(false); //collapse the range to the end point. false means collapse to end rather than the start
         range.select(); //Select the range (make it the visible selection
     }
 }


 // websockets 
 var socket;
 const socketMessageListener = (event) => {
     console.log(event);
     JD.serverResponse(event.data);
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

 var JD = {};
 JD.debounce = function (func, wait, immediate) {
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

 JD.contentEdited = function () {
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
     document.getElementById("saved").style.display = 'inline-block';
     setTimeout(function(){ document.getElementById("saved").style.display = 'none'; }, 1000);
 };

 JD.serverResponse = function (jsonString) {
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
     }
 }

 JD.editClick = function (e) {
     e.preventDefault();
     JD.loadEditor();
 }

 JD.loadEditor = function () {
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

 document.getElementById("editable").addEventListener('input', JD.debounce(JD.contentEdited, 300));

 editlink = document.getElementById("editlink")
 if (editlink != null) {
     editlink.addEventListener("click", JD.loadEditor);
 }

//  // allow only pasting plain text
//  document.getElementById("editable").addEventListener("paste", function (e) {
//      e.preventDefault();
//      var text = e.clipboardData.getData("text/plain");
//      document.execCommand("insertHTML", false, text);
//  });

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
	var height = parseInt(computed.getPropertyValue('border-top-width'), 10)
	             + parseInt(computed.getPropertyValue('padding-top'), 10)
	             + field.scrollHeight
	             + parseInt(computed.getPropertyValue('padding-bottom'), 10)
	             + parseInt(computed.getPropertyValue('border-bottom-width'), 10);

	field.style.height = height + 'px';

};

document.getElementById("editable").addEventListener('input', function (event) {
	if (event.target.tagName.toLowerCase() !== 'textarea') return;
	autoExpand(event.target);
}, false);


 // if editing, go to edit page
 if (getParameterByName("edit") != null) {
     JD.loadEditor();
     document.getElementById("editable").focus();
     history.pushState({}, window.location.pathname, window.location.pathname);
 }
 
 
 