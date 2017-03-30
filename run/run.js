(function() {
	// some of these are also defined in defs.go, defs.h and work.js

	const MAX_PACKET_SIZE = 0x10000
	const MAX_SERVICES    = 100

	const PACKET_HEADER_SIZE         = 8
	const SERVICE_PACKET_HEADER_SIZE = PACKET_HEADER_SIZE + 8
	const SERVICE_INFO_SIZE          = 8

	const PACKET_FLAG_POLLOUT = 0x1

	let Gate = {
		scriptUrl: "../",
		ioUrl:     window.location.origin + "/io/",
		debug:     !!window.console,
		Runner:    Runner,
	}

	function Runner(wasm, serviceRegistry) {
		let runner = this
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

		function send(data) {
			if (sendBuffer === null)
				socket.send(data)
			else
				sendBuffer.push(data)
		}

		var pollout = false

		function sendPollout() {
			let buf = new ArrayBuffer(PACKET_HEADER_SIZE)
			let header = new DataView(buf, 0, PACKET_HEADER_SIZE)
			header.setUint32(0, buf.byteLength, true)
			header.setUint16(4, PACKET_FLAG_POLLOUT, true)
			socket.send(buf)
			pollout = false
		}

		let messenger = serviceRegistry.createMessenger((buf) => {
			if (buf.byteLength < PACKET_HEADER_SIZE || buf.byteLength > MAX_PACKET_SIZE)
				throw "invalid outgoing message packet buffer length"

			let header = new DataView(buf)

			if (header.getUint16(6, true) == 0)
				throw "service code is zero in outgoing message packet header"

			header.setUint32(0, buf.byteLength, true)
			header.setUint16(4, 0, true)

			send(buf)
		})

		function handleServices(requestBuf) {
			if (requestBuf.byteLength < SERVICE_PACKET_HEADER_SIZE)
				throw "service discovery packet is too short"

			let request = new DataView(requestBuf)
			let count = request.getUint32(PACKET_HEADER_SIZE+4, true)
			if (count > MAX_SERVICES)
				throw "too many services requested"

			let responseBuf = new ArrayBuffer(SERVICE_PACKET_HEADER_SIZE + SERVICE_INFO_SIZE*count)
			let response = new DataView(responseBuf)
			response.setUint32(0, responseBuf.byteLength, true)
			response.setUint32(PACKET_HEADER_SIZE+4, count, true)

			var nameBuf = new Uint8Array(requestBuf, SERVICE_PACKET_HEADER_SIZE)
			var responseOffset = SERVICE_PACKET_HEADER_SIZE

			for (var i = 0; i < count; i++) {
				let nameLen = nameBuf.indexOf(0)
				if (nameLen < 0)
					throw "name string is truncated in service discovery packet"

				var name = ""
				for (var j = 0; j < nameLen; j++)
					name += String.fromCharCode(nameBuf[j])

				nameBuf = nameBuf.slice(nameLen+1)

				let {code, version} = serviceRegistry.getInfo(name)
				response.setUint16(responseOffset + 0, code, true)
				response.setUint32(responseOffset + 4, version, true)
				responseOffset += SERVICE_INFO_SIZE
			}

			send(responseBuf)
		}

		socket.onmessage = (event) => {
			let buf = event.data
			let header = new DataView(buf)
			let size = header.getUint32(0, true)

			if (size != buf.byteLength)
				throw "inconsistent incoming packet size"

			let flags = header.getUint16(4, true)
			let code = header.getUint16(6, true)

			if ((flags & PACKET_FLAG_POLLOUT) != 0 && !pollout) {
				pollout = true
				setTimeout(sendPollout)
			}

			if (code === 0) {
				if (size > PACKET_HEADER_SIZE)
					handleServices(buf)
			} else {
				messenger.handleMessage(buf)
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
