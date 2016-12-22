#!/usr/bin/env python

from __future__ import print_function

import argparse
import contextlib
import socket
import struct
import sys


def main():
	parser = argparse.ArgumentParser()
	parser.add_argument("host")
	parser.add_argument("port", type=int)
	parser.add_argument("wasm")
	args = parser.parse_args()

	addr = (args.host, args.port)

	with open(args.wasm, "rb") as f:
		wasm = f.read()

	with contextlib.closing(socket.create_connection(addr)) as sock:
		sock.send(b"GET /execute-custom HTTP/1.0\r\n\r\n")

		resp = b""
		while True:
			b = sock.recv(1)
			if not b:
				print("EOF while receiving HTTP response resp", file=sys.stderr)
				sys.exit(1)
			resp += b
			if resp.endswith(b"\r\n\r\n"):
				break

		sock.send(struct.pack("<Q", len(wasm)))
		sock.send(wasm)
		sock.send(sys.stdin.read().encode())

		while True:
			data = sock.recv(4096)
			if not data:
				break
			sys.stdout.write(data)


if __name__ == "__main__":
	main()
