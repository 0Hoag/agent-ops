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
            if self.path.endswith("/issues?per_page=100&page=1&state=all"):
                body = json.dumps([{"id": 90401, "iid": 904, "title": "Smoke mapped GitLab issue", "state": "opened", "updated_at": "2026-01-01T00:00:00Z", "labels": ["Status::Review"]}]).encode()
            else:
                body = b"[]"
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("X-Next-Page", "")
            self.end_headers()
            self.wfile.write(body)
            return
        self.send_error(404)

    def do_POST(self):
        with open(log_path, "a", encoding="utf-8") as log:
            log.write(json.dumps({"method": "POST", "path": self.path, "token": self.headers.get("PRIVATE-TOKEN")}) + "\n")
        self.send_response(201)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"id":123,"iid":2,"title":"encrypted fixture","state":"opened","updated_at":"2026-01-01T00:00:00Z"}')

    def log_message(self, *_args):
        pass


HTTPServer(("0.0.0.0", port), Handler).serve_forever()
