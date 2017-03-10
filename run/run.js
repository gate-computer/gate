(function() {
	const OP_CODE_NONE       = 0
	const OP_CODE_ORIGIN     = 1
	const OP_CODE_INTERFACES = 2

	const OP_FLAG_POLLOUT = 0x1

	const OP_HEADER_SIZE = 8

	const EV_CODE_POLLOUT    = 0
	const EV_CODE_ORIGIN     = 1
	const EV_CODE_INTERFACES = 2

	const EV_HEADER_SIZE = 8

	let Gate = {
		scriptUrl: "../",
		ioUrl:     window.location.origin + "/io/",
		debug:     !!window.console,
		Runner:    Runner,
	}

	function Runner(wasm) {
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

		function sendPacket(packet) {
			if (sendBuffer === null)
				socket.send(packet)
			else
				sendBuffer.push(packet)
		}

		runner.sendOrigin = (payload) => {
			let packet = new ArrayBuffer(EV_HEADER_SIZE + payload.byteLength)
			let header = new DataView(packet, 0, EV_HEADER_SIZE)
			header.setUint32(0, packet.byteLength, true)
			header.setUint16(4, EV_CODE_ORIGIN, true)
			new Uint8Array(packet, EV_HEADER_SIZE).set(payload)
			sendPacket(packet)
		}

		var pollout = false

		function sendPollout() {
			let packet = new ArrayBuffer(EV_HEADER_SIZE)
			let header = new DataView(packet, 0, EV_HEADER_SIZE)
			header.setUint32(0, packet.byteLength, true)
			header.setUint16(4, EV_CODE_POLLOUT, true)
			socket.send(packet)
			pollout = false
		}

		socket.onmessage = (event) => {
			let header = new DataView(event.data, 0, OP_HEADER_SIZE)
			let size = header.getUint32(0, true)
			let code = header.getUint16(4, true)
			let flags = header.getUint16(6, true)

			if (size != event.data.byteLength)
				throw "inconsistent packet size"

			if ((flags & OP_FLAG_POLLOUT) != 0 && !pollout) {
				setTimeout(sendPollout)
				pollout = true
			}

			let payload = event.data.slice(OP_HEADER_SIZE)

			switch (code) {
			case OP_CODE_NONE:
				break

			case OP_CODE_ORIGIN:
				if (runner.onorigin)
					runner.onorigin(payload)
				break

			case OP_CODE_INTERFACES:
				let packet = new ArrayBuffer(EV_HEADER_SIZE + 4)
				let header = new DataView(packet, 0, EV_HEADER_SIZE)
				header.setUint32(0, packet.byteLength, true)
				header.setUint16(4, EV_CODE_INTERFACES, true)
				sendPacket(packet)
				break

			default:
				throw code
			}
		}

		let worker = new Worker(Gate.scriptUrl + "run/work.js")

		worker.onmessage = (event) => {
			worker.terminate()

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
