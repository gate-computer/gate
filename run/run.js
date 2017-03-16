(function() {
	// some of these are also defined in defs.go, defs.h and work.js

	const MAX_PACKET_SIZE = 0x10000
	const MAX_SERVICES    = 100

	const HEADER_SIZE          = 8
	const SERVICES_HEADER_SIZE = HEADER_SIZE + 8
	const MESSAGE_HEADER_SIZE  = HEADER_SIZE + 4

	const OP_CODE_NONE     = 0
	const OP_CODE_ORIGIN   = 1
	const OP_CODE_SERVICES = 2
	const OP_CODE_MESSAGE  = 3

	const OP_FLAG_POLLOUT = 0x1

	const EV_CODE_POLLOUT  = 0
	const EV_CODE_ORIGIN   = 1
	const EV_CODE_SERVICES = 2
	const EV_CODE_MESSAGE  = 3

	let Gate = {
		scriptUrl: "../",
		ioUrl:     window.location.origin + "/io/",
		debug:     !!window.console,
		Runner:    Runner,
	}

	function Runner(wasm, serviceRegistry) {
		let runner = this
		runner.onorigin = null
		runner.onexit = null

		let token = Math.random().toString()
		let socket = new WebSocket(Gate.ioUrl.replace(/^http/, "ws") + "run?" + token)
		socket.binaryType = "arraybuffer"

		var sendBuffer = []

		socket.onopen = (event) => {
			for (var i in sendBuffer)
				socket.send(sendBuffer[i])

			sendBuffer = null
		}

		function send(ev) {
			if (sendBuffer === null)
				socket.send(ev)
			else
				sendBuffer.push(ev)
		}

		var pollout = false

		function sendPollout() {
			let ev = new ArrayBuffer(HEADER_SIZE)
			let header = new DataView(ev, 0, HEADER_SIZE)
			header.setUint32(0, ev.byteLength, true)
			header.setUint16(4, EV_CODE_POLLOUT, true)
			socket.send(ev)
			pollout = false
		}

		runner.sendOrigin = (payload) => {
			let ev = new ArrayBuffer(HEADER_SIZE + payload.byteLength)
			let header = new DataView(ev)
			header.setUint32(0, ev.byteLength, true)
			header.setUint16(4, EV_CODE_ORIGIN, true)
			new Uint8Array(ev, HEADER_SIZE).set(payload)
			send(ev)
		}

		let messenger = serviceRegistry.createMessenger((ev) => {
			if (ev.byteLength < MESSAGE_HEADER_SIZE || ev.byteLength > MAX_PACKET_SIZE)
				throw "invalid ev packet buffer length"

			let header = new DataView(packet)
			header.setUint32(ev.byteLength, true)
			header.setUint16(EV_CODE_MESSAGE, true)
			header.setUint16(0, true)
			send(ev)
		})

		function handleServices(op) {
			if (op.byteLength < SERVICES_HEADER_SIZE)
				throw "services op: packet is too short"

			let count = op.getUint32(HEADER_SIZE, true)
			if (count > MAX_SERVICES)
				throw "services op: too many services requested"

			let evBuf = new ArrayBuffer(SERVICES_HEADER_SIZE + 8*count)
			let ev = new DataView(evBuf)
			ev.setUint32(0, evBuf.byteLength, true)
			ev.setUint16(4, EV_CODE_SERVICES, true)
			ev.setUint32(HEADER_SIZE, count, true)

			var nameBuf = new Uint8Array(op.buffer, SERVICES_HEADER_SIZE)
			var evOffset = SERVICES_HEADER_SIZE

			for (var i = 0; i < count; i++) {
				let nameLen = nameBuf.indexOf(0)
				if (nameLen < 0)
					throw "services op: name data is truncated"

				var name = ""
				for (var j = 0; j < nameLen; j++)
					name += String.fromCharCode(nameBuf[j])

				nameBuf = nameBuf.slice(nameLen+1)

				let {atom, version} = serviceRegistry.getInfo(name)
				ev.setUint32(evOffset + 0, atom, true)
				ev.setUint32(evOffset + 4, version, true)
				evOffset += 8
			}

			send(evBuf)
		}

		function handleMessage(op) {
			if (opBuf.byteLength < MESSAGE_HEADER_SIZE)
				throw "message op: packet is too short"

			if (!messenger.handleMessage(op.buffer))
				throw "message op: invalid service atom"
		}

		socket.onmessage = (event) => {
			let opBuf = event.data
			let op = new DataView(opBuf)
			let size = op.getUint32(0, true)

			if (size != opBuf.byteLength)
				throw "inconsistent op packet size"

			let code = op.getUint16(4, true)
			let flags = op.getUint16(6, true)

			if ((flags & OP_FLAG_POLLOUT) != 0 && !pollout) {
				pollout = true
				setTimeout(sendPollout)
			}

			switch (code) {
			case OP_CODE_NONE:
				break

			case OP_CODE_ORIGIN:
				if (runner.onorigin)
					runner.onorigin(opBuf.slice(HEADER_SIZE))
				break

			case OP_CODE_SERVICES:
				handleServices(op)
				break

			case OP_CODE_MESSAGE:
				handleMessage(op)
				break

			default:
				throw code
			}
		}

		let worker = new Worker(Gate.scriptUrl + "run/work.js")

		worker.onmessage = (event) => {
			messenger.shutdown()
			worker.terminate()
			socket.close()

			if ("gateResult" in event.data) {
				if (runner.onexit)
					runner.onexit(event.data.gateResult)
			} else {
				throw event.data
			}
		}

		worker.postMessage({
			wasm,
			token,
			ioUrl: Gate.ioUrl,
			debug: Gate.debug,
		})

		runner.run = () => {
			worker.postMessage({})
		}

		return runner
	}

	window.Gate = Gate
}())
