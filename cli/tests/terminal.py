"""Real PTY acceptance: arrow selection, defaults, editing and terminal cleanup."""
import fcntl
import json
import os
import pty
import select
import struct
import subprocess
import sys
import termios
import time

class Session:
    def __init__(self, scenario, width=80, height=24):
        self.master, self.slave = pty.openpty()
        self.original = termios.tcgetattr(self.slave)
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack('HHHH', height, width, 0, 0))
        self.data = b''
        self.process = subprocess.Popen(
            [sys.argv[1], '-test.run=^TestPromptPTYChild$'],
            stdin=self.slave, stderr=self.slave, stdout=subprocess.PIPE,
            env=dict(os.environ, JANA_PROMPT_TEST_CHILD='1', JANA_PROMPT_SCENARIO=scenario, TERM='xterm-256color'),
        )
    def wait(self, text):
        expected = text.encode()
        deadline = time.monotonic() + 5
        while expected not in self.data:
            if time.monotonic() >= deadline:
                raise AssertionError(('missing terminal frame', text, self.data))
            if select.select([self.master], [], [], .1)[0]:
                self.data += os.read(self.master, 65536)
    def send(self, value):
        os.write(self.master, value.encode())
    def finish(self, canceled=False):
        try:
            output = self.process.communicate(timeout=5)[0]
            while select.select([self.master], [], [], .02)[0]:
                self.data += os.read(self.master, 65536)
            assert (self.process.returncode != 0) == canceled, (output, self.data)
            assert termios.tcgetattr(self.slave) == self.original, 'terminal state was not restored'
            assert b'saved-secret' not in self.data, 'saved password leaked to terminal'
            return json.loads(output)
        finally:
            if self.process.poll() is None:
                self.process.kill()
                self.process.wait()
            os.close(self.master)
            os.close(self.slave)

session = Session('wizard')
session.wait('localhost')
session.send('перри\x1b[D\x1b[D-\r')
session.wait('public.orders')
session.send('\x1b[B\r')
session.wait('delivery_address')
session.send('\x1b[A\r')
session.wait('Enter — оставить сохранённый пароль')
session.send('privateX\x7f\x1b[H\x1b[3~P\r')
result = session.finish()
assert result == {'host': 'пер-ри', 'table': 2, 'column': 0, 'password': 'Private'}, result
assert b'privateX' not in session.data and b'Private' not in session.data, 'typed password leaked'
assert b'\r\n' in session.data, 'raw terminal line endings missing'

# Enter alone accepts saved text/password; a long list scrolls to the selection
# instead of dumping all entries into the terminal history.
session = Session('password-default')
session.wait('Пароль')
session.send('\r')
assert session.finish()['password'] == 'saved-secret'
session = Session('long', width=32, height=9)
session.wait('table_25')
assert b'table_00' not in session.data, 'long select did not scroll to saved default'
session.send('\x1b[F\x1b[A\r')
assert session.finish()['selected'] == 38
for scenario in ['cancel-select', 'cancel-password']:
    session = Session(scenario)
    session.wait('Таблица' if scenario == 'cancel-select' else 'Пароль')
    session.send('\x03')
    assert session.finish(canceled=True)['error'] == 'context canceled'
print('PTY prompts: arrows, defaults, Unicode editing, hidden passwords, long lists, Ctrl+C and terminal restore passed.')
