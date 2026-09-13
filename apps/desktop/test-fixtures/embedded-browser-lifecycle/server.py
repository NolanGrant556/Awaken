from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from threading import Thread
class Handler(BaseHTTPRequestHandler):
 def do_GET(self):
  page='B' if self.path.startswith('/b') else 'A'
  other='A' if page=='B' else 'B'
  body=f'''<!doctype html><html><meta charset="utf-8"><title>Awaken Browser {page}</title><body style="background:{'#e2edff' if page=='A' else '#ffeddd'};color:#17324f;font:24px system-ui;padding:32px"><h1>公开网页 {page}</h1><p>Awaken 浏览器本地隔离验收</p><a href="/{other.lower()}">同站转到 {other}</a><br><a href="http://127.0.0.1:18759/b">跨站转到 B</a><br><input aria-label="测试输入" placeholder="测试输入"><button>测试按钮</button></body></html>'''.encode()
  self.send_response(200);self.send_header('Content-Type','text/html; charset=utf-8');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
 def log_message(self,*args): pass
for port in (18758,18759):
 server=ThreadingHTTPServer(('127.0.0.1',port),Handler)
 Thread(target=server.serve_forever,daemon=True).start()
print('Fixture servers: 127.0.0.1:18758, 127.0.0.1:18759',flush=True)
Thread().join() if False else __import__('threading').Event().wait()
