'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const express = require('../index.js');

const { PassThrough } = require('node:stream');
const { IncomingMessage, ServerResponse } = require('node:http');

// Sem loopback no sandbox do oráculo: a requisição entra direto no app, com objetos
// reais de requisição/resposta do Node, e a resposta HTTP crua é lida de um socket fictício.
function dispatch(app, headers) {
  return new Promise((resolve, reject) => {
    const socket = new PassThrough();
    socket.remoteAddress = '127.0.0.1';
    let raw = '';
    socket.on('data', (chunk) => { raw += chunk.toString('utf8'); });
    socket.on('end', () => {
      const sep = raw.indexOf('\r\n\r\n');
      resolve({ status: Number(raw.slice(0, sep).split(' ')[1]), body: raw.slice(sep + 4) });
    });
    const req = new IncomingMessage(socket);
    req.method = 'GET';
    req.url = '/';
    req.headers = headers;
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

async function seen(trust, headers) {
  const app = express();
  app.set('trust proxy', trust);
  app.get('/', (req, res) => res.json({
    host: req.host,
    hostname: req.hostname,
    subdomains: req.subdomains,
  }));
  const r = await dispatch(app, headers);
  assert.equal(r.status, 200);
  return JSON.parse(r.body);
}

test('H3 primeiro valor de X-Forwarded-Host com varios valores', async () => {
  assert.equal((await seen(true, { host: 'app.example.com', 'x-forwarded-host': 'a.example.com, b.example.com' })).host, 'a.example.com');
  assert.equal((await seen(true, { host: 'app.example.com', 'x-forwarded-host': 'a.example.com,b.example.com' })).host, 'a.example.com');
  assert.equal((await seen(true, { host: 'app.example.com', 'x-forwarded-host': 'a.example.com:8080, b.example.com' })).host, 'a.example.com:8080');
});

test('H3 valor vazio no inicio do X-Forwarded-Host cai para o Host', async () => {
  assert.equal((await seen(true, { host: 'app.example.com', 'x-forwarded-host': ', b.example.com' })).host, 'app.example.com');
  assert.equal((await seen(true, { host: 'app.example.com', 'x-forwarded-host': ' , b.example.com' })).host, 'app.example.com');
  assert.equal((await seen(true, { host: 'app.example.com', 'x-forwarded-host': ',' })).host, 'app.example.com');
  assert.equal((await seen(true, { host: 'app.example.com', 'x-forwarded-host': '   ' })).host, 'app.example.com');
});

test('H3 sem proxy confiavel o X-Forwarded-Host e ignorado', async () => {
  assert.equal((await seen(false, { host: 'app.example.com', 'x-forwarded-host': 'evil.test' })).host, 'app.example.com');
  assert.equal((await seen(false, { host: 'app.example.com', 'x-forwarded-host': ', evil.test' })).host, 'app.example.com');
});

test('H3 sem X-Forwarded-Host e sem Host devolve undefined', async () => {
  const j = await seen(true, {});
  assert.equal(j.host, undefined);
  assert.equal(j.hostname, undefined);
});

test('H3 hostname e subdomains seguem o host escolhido', async () => {
  const j = await seen(true, { host: 'app.example.com', 'x-forwarded-host': 'api.shop.example.com, proxy.test' });
  assert.equal(j.hostname, 'api.shop.example.com');
  assert.deepEqual(j.subdomains, ['shop', 'api']);

  const k = await seen(true, { host: 'app.example.com', 'x-forwarded-host': ', proxy.test' });
  assert.equal(k.hostname, 'app.example.com');
  assert.deepEqual(k.subdomains, ['app']);
});

test('H3 hostname continua tirando a porta do host escolhido', async () => {
  const j = await seen(true, { host: 'app.example.com', 'x-forwarded-host': 'api.example.com:8443, proxy.test' });
  assert.equal(j.hostname, 'api.example.com');
  const k = await seen(true, { host: 'app.example.com:3000' });
  assert.equal(k.host, 'app.example.com:3000');
  assert.equal(k.hostname, 'app.example.com');
});

test('H3 valor unico do X-Forwarded-Host continua igual', async () => {
  assert.equal((await seen(true, { host: 'app.example.com', 'x-forwarded-host': 'a.example.com' })).host, 'a.example.com');
  assert.equal((await seen(true, { host: 'app.example.com', 'x-forwarded-host': 'a.example.com:9000' })).hostname, 'a.example.com');
});
