"""Loopback model fixture for native Desktop acceptance; logs counts, never prompts or answers."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import argparse
import json
import time
import threading

parser = argparse.ArgumentParser()
parser.add_argument('--port', type=int, default=39177)
parser.add_argument('--log', default='/tmp/awaken-reliability-model.jsonl')
args = parser.parse_args()
lock = threading.Lock()

def tool_call(call_id, name, arguments):
    return {'id': call_id, 'type': 'function', 'function': {'name': name, 'arguments': json.dumps(arguments, ensure_ascii=False)}}

def find_task(value):
    if isinstance(value, dict):
        if value.get('task_id'): return value['task_id']
        for child in value.values():
            found = find_task(child)
            if found: return found
    if isinstance(value, list):
        for child in value:
            found = find_task(child)
            if found: return found
    if isinstance(value, str) and value.startswith('{'):
        try: return find_task(json.loads(value))
        except ValueError: pass
    return None

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_): pass
    def do_POST(self):
        data = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        messages, tools = data.get('messages', []), data.get('tools', [])
        mode = self.path.split('/')[1]
        results = [message for message in messages if message.get('role') == 'tool']
        child = any('CHILD_FIXTURE' in str(message.get('content','')) for message in messages if message.get('role') == 'user')
        with lock, open(args.log, 'a') as log:
            log.write(json.dumps({'at': time.time(), 'mode': mode, 'child': child, 'tool_result_count': len(results), 'has_tools': bool(tools), 'nudge': 'awaken_runtime_notice' in json.dumps(messages)})+'\n')
        content, call = '受控验收完成', None
        if not tools: content = '受控可靠性验收'
        elif mode == 'ask' and not results:
            call = tool_call('fixture-question','ask_user',{'questions':[{'question':'停止后保留原问题一？','options':[{'label':'A'},{'label':'B'}]},{'question':'停止后保留原问题二？','options':[{'label':'C'},{'label':'D'}]}]})
        elif mode in ('parent','tree'):
            if child and not results:
                if mode == 'tree':
                    time.sleep(2)
                    call = tool_call('child-hold','mcp__reliability_local__hold',{})
                else: call = tool_call('child-sleep','Bash',{'command':'sleep 60','description':'受控子任务等待'})
            elif not child:
                if not results or (mode == 'tree' and len(results) == 1):
                    call = tool_call('spawn-'+str(len(results)),'subagent',{'action':'spawn','description':'受控等待子任务 '+str(len(results)+1),'prompt':'CHILD_FIXTURE: 仅运行受控等待工具，不访问其他资源。','agent_type':'general','workspace_mode':'shared'})
                elif mode == 'parent' and len(results) == 1:
                    time.sleep(2)
                    call = tool_call('cancel-child','subagent',{'action':'cancel','task_id':find_task(results),'reason':'fixture-parent-stop'})
                elif mode == 'tree' and len(results) == 2:
                    call = tool_call('root-hold','mcp__reliability_local__hold',{})
        elif mode == 'progress' and len(results) < 12:
            call = tool_call('progress-'+str(len(results)), 'Bash', {'command':'sleep 8 && printf fixture', 'description':'受控进展工具 '+str(len(results)+1)})
            content = '已完成前八项受控检查，接下来继续剩余四项。' if 'awaken_runtime_notice' in json.dumps(messages) else ''
        elif mode == 'tools' and not results:
            call = tool_call('fixture-slow-tool','Bash',{'command':'sleep 60','description':'受控慢工具'})
        self.send_response(200)
        self.send_header('Content-Type','text/event-stream')
        self.end_headers()
        def chunk(delta, finish=None):
            self.wfile.write(('data: '+json.dumps({'id':'fixture-message','object':'chat.completion.chunk','created':int(time.time()),'model':'fixture','choices':[{'index':0,'delta':delta,'finish_reason':finish}]},ensure_ascii=False)+'\n\n').encode())
            self.wfile.flush()
        try:
            if tools and mode in ('stream','disconnect'):
                chunk({'role':'assistant','content':'受控模型正在输出，等待停止。'})
                if mode == 'disconnect': self.close_connection=True; return
                for _ in range(90): time.sleep(1); chunk({'content':' '})
            elif call:
                if mode == 'progress' and content: chunk({'role':'assistant','content':content})
                chunk({'role':'assistant','tool_calls':[{'index':0,**call}]})
            else: chunk({'role':'assistant','content':content})
            chunk({},'tool_calls' if call else 'stop')
            self.wfile.write(b'data: [DONE]\n\n'); self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError): pass

ThreadingHTTPServer(('127.0.0.1',args.port), Handler).serve_forever()
