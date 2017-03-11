(function() {
	const MAX_INTERFACES = 100

	const OP_CODE_NONE       = 0
	const OP_CODE_ORIGIN     = 1
	const OP_CODE_INTERFACES = 2
	const OP_CODE_MESSAGE    = 3

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

	function Runner(wasm, ifaces) {
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
			let op = new DataView(event.data)
			let size = op.getUint32(0, true)
			let code = op.getUint16(4, true)
			let flags = op.getUint16(6, true)

			if (size != op.byteLength)
				throw "inconsistent op packet size"

			if ((flags & OP_FLAG_POLLOUT) != 0 && !pollout) {
				setTimeout(sendPollout)
				pollout = true
			}

			switch (code) {
			case OP_CODE_NONE:
				break

			case OP_CODE_ORIGIN:
				if (runner.onorigin)
					runner.onorigin(event.data.slice(OP_HEADER_SIZE))
				break

			case OP_CODE_INTERFACES:
				if (op.byteLength < OP_HEADER_SIZE + 4 + 4)
					throw "interfaces op: packet is too short"

				let count = op.getUint32(OP_HEADER_SIZE, true)
				if (count > MAX_INTERFACES)
					throw "interfaces op: too many interfaces requested"

				let evPacket = new ArrayBuffer(EV_HEADER_SIZE + 4 + 4 + 8*count)
				let ev = new DataView(evPacket)
				ev.setUint32(0, evPacket.byteLength, true)
				ev.setUint16(4, EV_CODE_INTERFACES, true)
				ev.setUint32(EV_HEADER_SIZE, count, true)

				var nameBuf = new Uint8Array(event.data, OP_HEADER_SIZE + 4 + 4)
				var evOffset = EV_HEADER_SIZE + 4 + 4

				for (var i = 0; i < count; i++) {
					let nameLen = nameBuf.indexOf(0)
					if (nameLen < 0)
						throw "interfaces op: name data is truncated"

					var name = ""
					for (var j = 0; j < nameLen; j++)
						name += String.fromCharCode(nameBuf[j])

					nameBuf = nameBuf.slice(nameLen+1)

					let {atom, version} = ifaces.getInfo(name)
					ev.setUint32(evOffset + 0, atom, true)
					ev.setUint32(evOffset + 4, version, true)
					evOffset += 8
				}

				sendPacket(evPacket)
				break

			case OP_CODE_MESSAGE:
				if (op.byteLength < OP_HEADER_SIZE + 4)
					throw "message op: packet is too short"

				let atom = op.getUint32(OP_HEADER_SIZE, true)

				if (atom == 0 || !ifaces.onmessage(event.data.slice(OP_HEADER_SIZE), atom))
					throw "message op: invalid interface atom"

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
