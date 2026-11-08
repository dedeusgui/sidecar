'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const express = require('../index.js');

const { PassThrough } = require('node:stream');
const { IncomingMessage, ServerResponse } = require('node:http');

// Sem loopback no sandbox do oráculo: a requisição entra direto no app, com objetos
// reais de requisição/resposta do Node, e a resposta HTTP crua é lida de um socket fictício.
function dispatch(app, method, url, headers = {}) {
  return new Promise((resolve, reject) => {
    const socket = new PassThrough();
    socket.remoteAddress = '127.0.0.1';
    let raw = '';
    socket.on('data', (chunk) => { raw += chunk.toString('utf8'); });
    socket.on('end', () => {
      const sep = raw.indexOf('\r\n\r\n');
      const [statusLine, ...lines] = raw.slice(0, sep).split('\r\n');
      const out = {};
      for (const line of lines) {
        const i = line.indexOf(':');
        out[line.slice(0, i).trim().toLowerCase()] = line.slice(i + 1).trim();
      }
      resolve({ status: Number(statusLine.split(' ')[1]), headers: out, body: raw.slice(sep + 4) });
    });
    const req = new IncomingMessage(socket);
    req.method = method;
    req.url = url;
    req.headers = { host: 'x.test', ...headers };
    const res = new ServerResponse(req);
    res.assignSocket(socket);
    res.on('finish', () => socket.end());
    res.on('error', reject);
    try {
      app(req, res);
    } catch (err) {
      reject(err);
    }
  });
}

function jsonpApp() {
  const app = express();
  app.get('/', (req, res) => res.jsonp({ a: 1 }));
  return app;
}

test('EDIT01 callback formado so por caracteres invalidos responde JSON', async () => {
  for (const cb of ['!!!', '%20']) {
    const r = await dispatch(jsonpApp(), 'GET', '/?callback=' + cb);
    assert.equal(r.status, 200, 'status para callback=' + cb);
    assert.match(r.headers['content-type'], /^application\/json/, 'content-type para callback=' + cb);
    assert.deepEqual(JSON.parse(r.body), { a: 1 }, 'corpo para callback=' + cb);
  }
});

test('EDIT01 callback valido continua como script executavel', async () => {
  const r = await dispatch(jsonpApp(), 'GET', '/?callback=cb');
  assert.match(r.headers['content-type'], /^text\/javascript/);
  let got;
  vm.runInNewContext(r.body, { cb: (value) => { got = value; } });
  assert.equal(JSON.stringify(got), '{"a":1}');
});

test('EDIT01 callback com caracteres filtrados continua sendo sanitizado', async () => {
  const r = await dispatch(jsonpApp(), 'GET', '/?callback=foo(bar)');
  assert.match(r.headers['content-type'], /^text\/javascript/);
  let got;
  vm.runInNewContext(r.body, { foobar: (value) => { got = value; } });
  assert.equal(JSON.stringify(got), '{"a":1}');
});
