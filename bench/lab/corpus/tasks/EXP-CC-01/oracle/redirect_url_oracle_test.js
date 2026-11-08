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

// Deprecation notices go to process.stderr; capture them while fn runs.
async function captureStderr(fn) {
  const original = process.stderr.write;
  let captured = '';
  process.stderr.write = (chunk) => {
    captured += String(chunk);
    return true;
  };
  try {
    const result = await fn();
    return { result, captured };
  } finally {
    process.stderr.write = original;
  }
}

function redirectApp(handler) {
  const app = express();
  app.get('/', handler);
  return app;
}

test('CC01 URL aceito em redirect sem aviso de deprecacao', async () => {
  const app = redirectApp((req, res) => res.redirect(new URL('http://x.test/a')));
  const { result: r, captured } = await captureStderr(() => dispatch(app, 'GET', '/'));
  assert.equal(r.status, 302);
  assert.equal(r.headers.location, 'http://x.test/a');
  assert.match(r.body, /Redirecting to http:\/\/x\.test\/a/);
  assert.doesNotMatch(captured, /Url must be a string/);
});

test('CC01 URL aceito tambem na forma com status', async () => {
  const app = redirectApp((req, res) => res.redirect(301, new URL('http://x.test/b')));
  const { result: r, captured } = await captureStderr(() => dispatch(app, 'GET', '/'));
  assert.equal(r.status, 301);
  assert.equal(r.headers.location, 'http://x.test/b');
  assert.doesNotMatch(captured, /Url must be a string/);
});

test('CC01 string continua com corpo, tipo e Vary iguais', async () => {
  const r = await dispatch(redirectApp((req, res) => res.redirect('/x')), 'GET', '/');
  assert.equal(r.status, 302);
  assert.equal(r.headers.location, '/x');
  assert.match(r.headers['content-type'], /^text\/plain/);
  assert.equal(r.body, 'Found. Redirecting to /x');
  assert.match(r.headers.vary, /Accept/);
});

test('CC01 HTML e HEAD continuam iguais', async () => {
  const app = redirectApp((req, res) => res.redirect(301, '/y'));
  const html = await dispatch(app, 'GET', '/', { accept: 'text/html' });
  assert.equal(html.status, 301);
  assert.match(html.headers['content-type'], /^text\/html/);
  assert.match(html.body, /<p>Moved Permanently\. Redirecting to \/y<\/p>/);

  const head = await dispatch(app, 'HEAD', '/');
  assert.equal(head.status, 301);
  assert.equal(head.body, '');
});

test('CC01 tipo que nao e string nem URL continua com aviso de deprecacao', async () => {
  const app = redirectApp((req, res) => res.redirect(123));
  const { result: r, captured } = await captureStderr(() => dispatch(app, 'GET', '/'));
  assert.equal(r.status, 302);
  assert.equal(r.body, 'Found. Redirecting to 123');
  assert.match(captured, /Url must be a string/);
});
