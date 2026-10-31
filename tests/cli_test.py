"""Exercise the shipping CLI against a synthetic local PixelLab API."""
import base64
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile
import threading
import unittest
from urllib.parse import parse_qs, urlsplit

BINARY=Path(os.environ.get('PXLB_TEST_BINARY','target/debug/pxlb'+('.exe' if os.name=='nt' else ''))).resolve()
OPERATIONS=json.loads((Path(__file__).parent/'operations.json').read_text())


@contextmanager
def api(responder=None):
    calls=[]
    class Handler(BaseHTTPRequestHandler):
        def log_message(self,*_args):
            pass
        def respond(self):
            body=self.rfile.read(int(self.headers.get('Content-Length','0')))
            parts=urlsplit(self.path)
            calls.append(dict(method=self.command,path=parts.path,query=parse_qs(parts.query),auth=self.headers.get('Authorization'),body=json.loads(body) if body else None))
            status,payload,media=responder(calls) if responder else (200,dict(ok=True),'application/json')
            data=payload if isinstance(payload,bytes) else json.dumps(payload).encode()
            self.send_response(status)
            self.send_header('Content-Type',media)
            self.send_header('Content-Length',str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        do_GET=respond
        do_POST=respond
        do_PATCH=respond
        do_DELETE=respond
    server=ThreadingHTTPServer(('127.0.0.1',0),Handler)
    thread=threading.Thread(target=lambda:server.serve_forever(poll_interval=.02),daemon=True)
    thread.start()
    try:
        yield f'http://127.0.0.1:{server.server_port}',calls
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


class CLI(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root=Path(self.temp.name)
        self.env=dict(os.environ,PIXELLAB_API_KEY='fixture-token')
    def run_cli(self,*args,**env):
        return subprocess.run([str(BINARY),*args],cwd=self.root,env=self.env|env,capture_output=True,text=True,timeout=10)
    def request(self,url,route,*flags,**env):
        return self.run_cli(route,'--base-url',url,*flags,**env)
    def test_all_64_operations_and_help_descriptions_are_preserved(self):
        help=self.run_cli('--help')
        self.assertEqual(help.returncode,0)
        with api() as (url,calls):
            for operation in OPERATIONS:
                method,template=operation.split(' ',1)
                route=re.sub(r'\{[^}]+\}','fixture',template)
                with self.subTest(operation=operation):
                    result=self.request(url,route,'--http-method',method,'--body-json','{}')
                    self.assertEqual(result.returncode,0,result.stderr)
                    self.assertEqual((calls[-1]['method'],calls[-1]['path']),(method,route))
                    self.assertEqual(calls[-1]['auth'],'Bearer fixture-token')
                    self.assertIn(re.sub(r'\{([^}]+)\}',r'<\1>',template).lstrip('/'),help.stdout)
        self.assertEqual(len(calls),64)
    def test_balance_is_get_without_body(self):
        with api() as (url,calls):
            self.assertEqual(self.request(url,'balance').returncode,0)
            self.assertEqual(calls[0]['method'],'GET')
            self.assertIsNone(calls[0]['body'])
    def test_body_fields_keep_sizes_booleans_numbers_arrays_and_json_types(self):
        with api() as (url,calls):
            result=self.request(url,'create-image-pixen','--description','forest','--image-size','64x32','--seed','5','--background','false','--custom-json','{"a":1}')
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual(calls[-1]['body'],dict(description='forest',image_size=dict(width=64,height=32),seed=5,background=False,custom=dict(a=1)))
            result=self.request(url,'objects/id/select-frames','--index','2','--index','4')
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual(calls[-1]['body'],dict(indices=[2,4]))
            result=self.request(url,'objects/id/tags','--tag','first','--tag','second')
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual(calls[-1]['body'],dict(tags=['first','second']))
    def test_safe_get_default_method_override_and_colliding_post_selection(self):
        with api() as (url,calls):
            self.assertEqual(self.request(url,'objects/id').returncode,0)
            self.assertEqual(calls[-1]['method'],'GET')
            self.assertEqual(self.request(url,'objects/id','--http-method','delete').returncode,0)
            self.assertEqual(calls[-1]['method'],'DELETE')
            self.assertEqual(self.request(url,'tilesets','--limit','2','--offset','3').returncode,0)
            self.assertEqual(calls[-1]['method'],'GET')
            self.assertEqual(calls[-1]['query'],dict(limit=['2'],offset=['3']))
            result=self.request(url,'tilesets','--lower-description','grass','--upper-description','rock')
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual(calls[-1]['method'],'POST')
    def test_invalid_routes_path_flags_methods_and_required_fields_never_submit(self):
        with api() as (url,calls):
            for route,flags in [('objects/{object_id}',[]),('objects/<object_id>',[]),('objects/id',['--object-id','id']),('balance',['--http-method','delete']),('create-image-pixen',['--description','forest']),('unknown',[])]:
                with self.subTest(route=route,flags=flags):
                    self.assertEqual(self.request(url,route,*flags).returncode,2)
            self.assertEqual(calls,[])
    def test_raw_json_inline_and_file_bypass_required_fields(self):
        (self.root/'body.json').write_text('{"nested":{"ok":true}}')
        with api() as (url,calls):
            for raw in ('{"nested":{"ok":true}}','body.json'):
                result=self.request(url,'create-image-pixen','--body-json',raw)
                self.assertEqual(result.returncode,0,result.stderr)
                self.assertEqual(calls[-1]['body'],dict(nested=dict(ok=True)))
    def test_image_paths_and_repeated_style_images_are_encoded(self):
        (self.root/'first.png').write_bytes(b'first-image')
        (self.root/'second.jpg').write_bytes(b'second-image')
        with api() as (url,calls):
            result=self.request(url,'generate-with-style-v2','--description','forest','--image-size','32','--style-image','first.png','--style-image','second.jpg')
            self.assertEqual(result.returncode,0,result.stderr)
            images=calls[-1]['body']['style_images']
            self.assertEqual([image['format'] for image in images],['png','jpeg'])
            self.assertEqual([base64.b64decode(image['base64']) for image in images],[b'first-image',b'second-image'])
    def test_nested_async_job_is_polled_and_images_are_saved_privately(self):
        def responder(calls):
            if len(calls)==1:
                return 200,dict(result=dict(background_job_id='job-1')),'application/json'
            if len(calls)==2:
                return 200,dict(status='processing'),'application/json'
            return 200,dict(status='completed',last_response=dict(images=[dict(base64=base64.b64encode(b'one').decode(),format='png'),dict(base64='data:image/png;base64,'+base64.b64encode(b'two').decode(),format='png')])),'application/json'
        with api(responder) as (url,calls):
            result=self.request(url,'generate-image-v2','--body-json','{}','--wait','--poll-interval','1ms','--timeout','1s','--out','images','--quiet')
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual(result.stdout,'')
            self.assertEqual([call['method'] for call in calls],['POST','GET','GET'])
            self.assertEqual(calls[-1]['path'],'/background-jobs/job-1')
            for index,content in enumerate((b'one',b'two')):
                file=self.root/f'images/image_{index:02}.png'
                self.assertEqual(file.read_bytes(),content)
                if os.name!='nt':
                    self.assertEqual(stat.S_IMODE(file.stat().st_mode),0o600)
    def test_timeout_and_api_failures_return_nonzero(self):
        with api(lambda calls:(200,dict(job_id='job-1') if len(calls)==1 else dict(status='processing'),'application/json')) as (url,_):
            result=self.request(url,'generate-image-v2','--body-json','{}','--wait','--poll-interval','1ms','--timeout','10ms')
            self.assertNotEqual(result.returncode,0)
            self.assertIn('timeout',result.stderr)
        with api(lambda _:(500,dict(detail='fixture failed'),'application/json')) as (url,_):
            result=self.request(url,'balance')
            self.assertEqual(result.returncode,1)
            self.assertIn('API error 500: fixture failed',result.stderr)
    def test_token_precedence_flag_environment_then_dotenv(self):
        (self.root/'.env').write_text('export PIXELLAB_API_KEY="file-token"\n')
        with api() as (url,calls):
            for flags,env,expected in [([],dict(PIXELLAB_API_KEY=''),'file-token'),([],dict(PIXELLAB_API_KEY='env-token'),'env-token'),(['--token','flag-token'],dict(PIXELLAB_API_KEY='env-token'),'flag-token')]:
                result=self.request(url,'balance',*flags,**env)
                self.assertEqual(result.returncode,0,result.stderr)
                self.assertEqual(calls[-1]['auth'],'Bearer '+expected)
                self.assertNotIn(expected,result.stdout+result.stderr)
    def test_binary_download_and_single_image_outfile(self):
        with api(lambda _:(200,b'fake-zip','application/zip')) as (url,_):
            result=self.request(url,'characters/id/zip','--out','files/result.zip')
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual((self.root/'files/result.zip').read_bytes(),b'fake-zip')
        with api(lambda _:(200,dict(base64=base64.b64encode(b'image').decode(),format='png'),'application/json')) as (url,_):
            result=self.request(url,'balance','--out','result.png')
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual((self.root/'result.png').read_bytes(),b'image')


if __name__=='__main__':
    unittest.main(verbosity=2)
