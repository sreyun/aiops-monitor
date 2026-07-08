import http.server, socketserver, sys

class NoCacheHandler(http.server.SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header("Cache-Control", "no-cache, no-store, must-revalidate")
        self.send_header("Pragma", "no-cache")
        self.send_header("Expires", "0")
        super().end_headers()

class ThreadingServer(socketserver.ThreadingMixIn, socketserver.TCPServer):
    daemon_threads = True
    allow_reuse_address = True

port = int(sys.argv[1]) if len(sys.argv) > 1 else 8090
with ThreadingServer(("", port), NoCacheHandler) as httpd:
    print(f"Serving at http://localhost:{port}")
    httpd.serve_forever()
