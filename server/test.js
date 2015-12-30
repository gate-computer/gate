(function() {
	function log(text) {
		var span = document.createElement('span');
		span.innerText = text;

		var div = document.getElementById('log');
		div.appendChild(span);
		div.scrollIntoView(false);
	}

	var r = new XMLHttpRequest();
	r.onloadstart = function() {
		log('load start');
	};
	r.onprogress = function(e) {
		log('progress: ' + e.loaded + ' bytes loaded');
	};
	r.onloadend = function() {
		log('load end: status: ' + r.status);
		log('load end: content: ' + JSON.stringify(r.response));
	};
	r.open("POST", "https://localhost:44321/");
	r.send("hello world");
}());
