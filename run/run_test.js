function testRun(url) {
	function handleOrigin(data) {
		let bytes = new Uint8Array(data)
		var text = ""

		for (var i = 0; i < bytes.length; i++)
			text += String.fromCharCode(bytes[i])

		console.log("origin:", bytes, text)
	}

	function handleExit(status) {
		console.log("exit:", status)
	}

	let xhr = new XMLHttpRequest()
	xhr.responseType = "arraybuffer"
	xhr.open("GET", url)
	xhr.send()
	xhr.onload = () => {
		let runner = new Gate.Runner(xhr.response)
		runner.onorigin = handleOrigin
		runner.onexit = handleExit
		runner.run()

		setTimeout(() => {
			let data = new DataView(new ArrayBuffer(1))
			data.setUint8(0, 43)
			runner.sendOrigin(data.buffer)
		}, 333)
	}
}
