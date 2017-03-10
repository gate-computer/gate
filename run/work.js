// some of these are also defined in defs.go and defs.h

const ABI_VERSION     = 0
const MAX_PACKET_SIZE = 0x10000

const RECV_FLAG_NONBLOCK = 0x1

function load({wasm, token, ioUrl, debug}) {
	let defaultUrl = ioUrl + "work?" + token
	let nonblockingUrl = ioUrl + "work/nonblock?" + token

	var memory
	var recvbuf = new ArrayBuffer()

	let env = {
		__gate_get_abi_version() {
			return ABI_VERSION
		},

		__gate_get_max_packet_size() {
			return MAX_PACKET_SIZE
		},

		__gate_func_ptr(id) {
			return 0
		},

		__gate_exit(status) {
			throw {gateResult: status}
		},

		__gate_recv(addr, size, flags) {
			let nonblock = (flags & RECV_FLAG_NONBLOCK) != 0
			let url = nonblock ? nonblockingUrl : defaultUrl

			while (recvbuf.byteLength == 0) {
				let xhr = new XMLHttpRequest()
				xhr.responseType = "arraybuffer"
				xhr.timeout = 50 * 1000
				xhr.open("GET", url, false)
				xhr.send()
				recvbuf = xhr.response

				if (nonblock && recvbuf.byteLength == 0)
					return size
			}

			var len = size
			if (recvbuf.byteLength < len)
				len = recvbuf.byteLength

			new Uint8Array(memory.buffer, addr).set(new Uint8Array(recvbuf, 0, len))
			recvbuf = recvbuf.slice(len)

			return size - len
		},

		__gate_send(addr, size) {
			let xhr = new XMLHttpRequest()
			xhr.open("POST", defaultUrl, false)
			xhr.send(new DataView(memory.buffer, addr, size))
		},
	}

	if (debug) {
		env.__gate_debug_write = (addr, size) => {
			let bytes = new Uint8Array(memory.buffer, addr, size)
			var text = ""

			for (var i = 0; i < size; i++)
				text += String.fromCharCode(bytes[i])

			console.log("debug:", text)
		}
	} else {
		env.__gate_debug_write = (addr, size) => {}
	}

	let module = new WebAssembly.Module(wasm)
	let instance = new WebAssembly.Instance(module, {env})

	memory = instance.exports.memory

	function run() {
		var msg
		try {
			let result = instance.exports.main()
			msg = {gateResult: result|0}
		} catch (e) {
			msg = e
		}
		postMessage(msg)
	}

	return run
}

onmessage = (event) => {
	let run = load(event.data)
	onmessage = () => run()
}
