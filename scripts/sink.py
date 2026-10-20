#!/usr/bin/env python3
"""Notification sink for the demo: prints each CAMARA CloudEvent it receives.

CAMARA requires an https sink, so this serves TLS with a self-signed
certificate created on first run (the gateway runs with -insecure-sink).
"""
import json
import pathlib
import ssl
import subprocess
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 9443
CERT = pathlib.Path(__file__).resolve().parent.parent / '.lab' / 'sink'


class Sink(BaseHTTPRequestHandler):
    def do_POST(self):
        event = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        auth = self.headers.get('Authorization', '-')
        print(f'{event["time"][11:19]}  {event["type"].rsplit(".", 1)[-1]}  ({auth[:13]}…)')
        print(json.dumps(event['data'], indent=2), flush=True)
        self.send_response(204)
        self.end_headers()

    def log_message(self, *args):
        pass


if not (CERT / 'cert.pem').exists():
    CERT.mkdir(parents=True, exist_ok=True)
    subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '365',
                    '-subj', '/CN=127.0.0.1', '-keyout', CERT / 'key.pem', '-out', CERT / 'cert.pem'],
                   check=True, capture_output=True)
server = HTTPServer(('127.0.0.1', PORT), Sink)
tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
tls.load_cert_chain(CERT / 'cert.pem', CERT / 'key.pem')
server.socket = tls.wrap_socket(server.socket, server_side=True)
print(f'sink listening on https://127.0.0.1:{PORT}/notifications', flush=True)
server.serve_forever()
