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

// `name` na query controla o argumento: ausente = chamada sem argumento.
function attachmentApp() {
  const app = express();
  app.get('/', (req, res) => {
    if (req.query.name === undefined) {
      res.attachment();
    } else {
      res.attachment(req.query.name);
    }
    res.end('x');
  });
  return app;
}

test('VOI01 attachment com string vazia equivale a chamar sem nome', async () => {
  const app = attachmentApp();
  const empty = await dispatch(app, 'GET', '/?name=');
  const none = await dispatch(app, 'GET', '/');
  assert.equal(none.headers['content-disposition'], 'attachment');
  assert.equal(empty.headers['content-disposition'], 'attachment');
  assert.equal(empty.headers['content-type'], none.headers['content-type']);
});

test('VOI01 attachment com nome normal continua igual', async () => {
  const r = await dispatch(attachmentApp(), 'GET', '/?name=relatorio.pdf');
  assert.equal(r.headers['content-disposition'], 'attachment; filename="relatorio.pdf"');
  assert.match(r.headers['content-type'], /^application\/pdf/);
});

test('VOI01 attachment com caminho usa so o nome do arquivo', async () => {
  const r = await dispatch(attachmentApp(), 'GET', '/?name=' + encodeURIComponent('/pasta/relatorio.pdf'));
  assert.equal(r.headers['content-disposition'], 'attachment; filename="relatorio.pdf"');
});
