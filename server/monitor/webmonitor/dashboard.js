(function() {
	const reconnectTimeout = 1000 // milliseconds

	let url = "ws://" + location.host + location.pathname + "websocket.json"

	function setConnected(yes) {
		document.getElementById("not-connected").style.display = yes ? "none" : "initial"
	}

	function log(text, color) {
		let row = document.createElement("tr")
		row.innerHTML = "<td>" + new Date().toString() + "</td><td>" + text + "</td>"
		row.style.color = color;
		document.getElementById("log").appendChild(row)
		row.scrollIntoView()
	}

	function connect(serverInit) {
		let conn = new WebSocket(url)

		conn.onmessage = (e) => {
			let frame = JSON.parse(e.data)
			let serverRestart = serverInit != 0 && frame.server_init != serverInit
			conn.onmessage = handler(serverRestart, frame.iface_types, frame.event_types, frame.state)
			serverInit = frame.server_init
			setConnected(true)
		}

		conn.onerror = (e) => {
			setConnected(false)
		}

		conn.onclose = (e) => {
			setConnected(false)
			setTimeout(() => connect(serverInit), reconnectTimeout)
		}
	}

	function handler(serverRestart, ifaceTypes, eventTypes, state) {
		function renderState(name) {
			document.getElementById(name).innerText = state[name]
		}

		if (!state.programs_loaded)
			state.programs_loaded = 0
		renderState("programs_loaded")

		if (!state.program_links)
			state.program_links = 0
		renderState("program_links")

		if (!state.instances)
			state.instances = 0
		renderState("instances")

		if (serverRestart)
			log("SERVER RESTART", "red")

		let ifaceNames = {}
		for (var name in ifaceTypes)
			ifaceNames[ifaceTypes[name]] = name

		function getIfaceName(context) {
			if (context && "iface" in context)
				return ifaceNames[context.iface]
			else
				return ""
		}

		function handleError(position, error) {
			log("Error: " + JSON.stringify(position) + ": " + error, "red")
		}

		function handleEvent(name, event, error, color) {
			var msg = name + " event: " + JSON.stringify(event)
			if (error)
				msg += ": " + error
			log(msg, color)
		}

		function eventHandler(name, color) {
			return (event, error) => {
				handleEvent(name, event, error, color)
			}
		}

		let eventHandlers = {}

		for (var name in eventTypes)
			eventHandlers[eventTypes[name]] = eventHandler(name)

		eventHandlers[eventTypes["ServerAccess"]] = eventHandler("ServerAccess", "grey")
		eventHandlers[eventTypes["FailNetwork"]] = eventHandler("FailNetwork", "yellow")
		eventHandlers[eventTypes["FailProtocol"]] = eventHandler("FailProtocol", "orange")
		eventHandlers[eventTypes["FailClient"]] = eventHandler("FailClient", "crimson")

		eventHandlers[eventTypes["ProgramLoad"]] = (event) => {
			handleEvent("ProgramLoad", event)
			state.programs_loaded++
			renderState("programs_loaded")
		}

		eventHandlers[eventTypes["ProgramCreate"]] = (event) => {
			handleEvent("ProgramCreate", event)
			state.program_links++
			renderState("program_links")
		}

		eventHandlers[eventTypes["InstanceCreate"]] = (event) => {
			handleEvent("InstanceCreate", event)
			state.instances++
			renderState("instances")
		}

		eventHandlers[eventTypes["InstanceDelete"]] = (event) => {
			handleEvent("InstanceDelete", event)
			state.instances--
			renderState("instances")
		}

		return (e) => {
			let frame = JSON.parse(e.data)

			if ("error" in frame) {
				let e = frame.error
				handleError(e.position, e.error)
			}

			if ("event" in frame) {
				let e = frame.event
				let handle = eventHandlers[e.type]
				if (handle)
					handle(e.event, e.error)
			}
		}
	}

	connect(0)
}())
