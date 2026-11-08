'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
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

function appWith(setup) {
  const app = express();
  app.get('/', (req, res) => {
    setup(res);
    res.send('ok');
  });
  return app;
}

test('DBG01 links vazios nao alteram Link existente', async () => {
  const r = await dispatch(appWith((res) => {
    res.links({ next: 'http://x.test/2' });
    res.links({});
  }), 'GET', '/');
  assert.equal(r.status, 200);
  assert.equal(r.headers.link, '<http://x.test/2>; rel="next"');
});

test('DBG01 links vazios sem Link previo nao criam cabecalho', async () => {
  const r = await dispatch(appWith((res) => {
    res.links({});
  }), 'GET', '/');
  assert.equal(r.status, 200);
  assert.equal(r.headers.link, undefined);
});

test('DBG01 links nao vazios continuam acumulando entradas', async () => {
  const r = await dispatch(appWith((res) => {
    res.links({ prev: 'http://x.test/1' });
    res.links({ next: 'http://x.test/3' });
  }), 'GET', '/');
  assert.equal(r.headers.link, '<http://x.test/1>; rel="prev", <http://x.test/3>; rel="next"');
});

test('DBG01 links com array de valores continua funcionando', async () => {
  const r = await dispatch(appWith((res) => {
    res.links({ next: ['http://x.test/2', 'http://x.test/3'] });
  }), 'GET', '/');
  assert.equal(r.headers.link, '<http://x.test/2>; rel="next", <http://x.test/3>; rel="next"');
});
