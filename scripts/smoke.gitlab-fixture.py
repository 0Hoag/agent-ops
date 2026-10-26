#!/usr/bin/env python3
"""Minimal isolated GitLab API/webhook fixture used by runtime smoke."""
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


port, log_path = int(sys.argv[1]), sys.argv[2]


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.end_headers()
            return
        with open(log_path, "a", encoding="utf-8") as log:
            log.write(json.dumps({"method": "GET", "path": self.path, "token": self.headers.get("PRIVATE-TOKEN")}) + "\n")
        if self.path.startswith("/api/v4/projects/"):
            body = b"[]"
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("X-Next-Page", "")
            self.end_headers()
            self.wfile.write(body)
            return
        self.send_error(404)

    def log_message(self, *_args):
        pass


HTTPServer(("0.0.0.0", port), Handler).serve_forever()
