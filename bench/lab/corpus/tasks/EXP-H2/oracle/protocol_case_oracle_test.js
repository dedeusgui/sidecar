'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const express = require('../index.js');

const { PassThrough } = require('node:stream');
const { IncomingMessage, ServerResponse } = require('node:http');

// Sem loopback no sandbox do oráculo: a requisição entra direto no app, com objetos
// reais de requisição/resposta do Node, e a resposta HTTP crua é lida de um socket fictício.
function dispatch(app, headers, { encrypted = false, remoteAddress = '127.0.0.1' } = {}) {
  return new Promise((resolve, reject) => {
    const socket = new PassThrough();
    socket.remoteAddress = remoteAddress;
    if (encrypted) socket.encrypted = true;
    let raw = '';
    socket.on('data', (chunk) => { raw += chunk.toString('utf8'); });
    socket.on('end', () => {
      const sep = raw.indexOf('\r\n\r\n');
      resolve({ status: Number(raw.slice(0, sep).split(' ')[1]), body: raw.slice(sep + 4) });
    });
    const req = new IncomingMessage(socket);
    req.method = 'GET';
    req.url = '/';
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

function appTrusting(trust) {
  const app = express();
  app.set('trust proxy', trust);
  app.get('/', (req, res) => res.json({ protocol: req.protocol, secure: req.secure }));
  return app;
}

async function seen(trust, forwardedProto, opts) {
  const headers = forwardedProto === undefined ? {} : { 'x-forwarded-proto': forwardedProto };
  const r = await dispatch(appTrusting(trust), headers, opts);
  assert.equal(r.status, 200);
  return JSON.parse(r.body);
}

test('H2 X-Forwarded-Proto em maiusculas sai em minusculas', async () => {
  assert.equal((await seen(true, 'HTTPS')).protocol, 'https');
  assert.equal((await seen(true, 'Http')).protocol, 'http');
});

test('H2 lista com maiusculas usa o primeiro valor em minusculas', async () => {
  assert.equal((await seen(true, 'Https, http')).protocol, 'https');
  assert.equal((await seen(true, 'HTTP,https')).protocol, 'http');
  assert.equal((await seen(true, 'HTTPS , http')).protocol, 'https');
});

test('H2 espacos ao redor do valor sao removidos', async () => {
  assert.equal((await seen(true, '  HTTPS  ')).protocol, 'https');
  assert.equal((await seen(true, ' Https ,http')).protocol, 'https');
});

test('H2 cabecalho ignorado quando o proxy nao e confiavel', async () => {
  assert.equal((await seen(false, 'HTTPS')).protocol, 'http');
  assert.equal((await seen(false, 'HTTPS')).secure, false);
  assert.equal((await seen(false, 'HTTP', { encrypted: true })).protocol, 'https');
});

test('H2 sem cabecalho usa o protocolo do socket', async () => {
  assert.equal((await seen(true, undefined, { encrypted: true })).protocol, 'https');
  assert.equal((await seen(true, undefined)).protocol, 'http');
  assert.equal((await seen(true, '', { encrypted: true })).protocol, 'https');
});

test('H2 valor ja minusculo continua igual', async () => {
  assert.equal((await seen(true, 'https')).protocol, 'https');
  assert.equal((await seen(true, 'http', { encrypted: true })).protocol, 'http');
  assert.equal((await seen(true, 'https, http')).secure, true);
});

test('H2 secure acompanha o protocolo normalizado', async () => {
  assert.equal((await seen(true, 'HTTPS')).secure, true);
  assert.equal((await seen(true, 'Https, http')).secure, true);
  assert.equal((await seen(true, 'HTTP')).secure, false);
});
