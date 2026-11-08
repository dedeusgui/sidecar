'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const express = require('../index.js');

const { PassThrough } = require('node:stream');
const { IncomingMessage, ServerResponse } = require('node:http');

// Sem loopback no sandbox do oráculo: a requisição entra direto no app, com objetos
// reais de requisição/resposta do Node, e a resposta HTTP crua é lida de um socket fictício.
function dispatch(app, headers, remoteAddress) {
  return new Promise((resolve, reject) => {
    const socket = new PassThrough();
    socket.remoteAddress = remoteAddress;
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

// Lança aqui se app.set rejeitar o valor, o que é o comportamento que o teste precisa observar.
async function ipFor(trust, remoteAddress, xForwardedFor) {
  const app = express();
  app.set('trust proxy', trust);
  app.get('/', (req, res) => res.json({ ip: req.ip }));
  const headers = xForwardedFor === undefined ? {} : { 'x-forwarded-for': xForwardedFor };
  const r = await dispatch(app, headers, remoteAddress);
  assert.equal(r.status, 200);
  return JSON.parse(r.body).ip;
}

const FORWARDED = '203.0.113.7';

test('H1 string vazia ou so espacos nao confia em proxy', async () => {
  assert.equal(await ipFor('', '127.0.0.1', FORWARDED), '127.0.0.1');
  assert.equal(await ipFor('   ', '127.0.0.1', FORWARDED), '127.0.0.1');
  assert.equal(await ipFor(' , ', '127.0.0.1', FORWARDED), '127.0.0.1');
});

test('H1 virgulas sobrando sao ignoradas na string', async () => {
  assert.equal(await ipFor('loopback,', '127.0.0.1', FORWARDED), FORWARDED);
  assert.equal(await ipFor(',loopback', '127.0.0.1', FORWARDED), FORWARDED);
  assert.equal(await ipFor('loopback,,10.0.0.0/8', '10.9.9.9', FORWARDED), FORWARDED);
  assert.equal(await ipFor('loopback,,10.0.0.0/8', '198.51.100.1', FORWARDED), '198.51.100.1');
});

test('H1 array com entrada vazia continua funcionando', async () => {
  assert.equal(await ipFor(['', 'loopback'], '127.0.0.1', FORWARDED), FORWARDED);
  assert.equal(await ipFor([''], '127.0.0.1', FORWARDED), '127.0.0.1');
  assert.equal(await ipFor(['loopback', ''], '127.0.0.1', FORWARDED), FORWARDED);
});

test('H1 espacos ao redor das entradas sao aparados', async () => {
  assert.equal(await ipFor(' loopback , , 10.0.0.0/8 ', '10.1.2.3', FORWARDED), FORWARDED);
  assert.equal(await ipFor(' loopback , , 10.0.0.0/8 ', '8.8.8.8', FORWARDED), '8.8.8.8');
});

test('H1 entrada invalida continua lancando erro', async () => {
  await assert.rejects(() => ipFor('nao-e-ip', '127.0.0.1', FORWARDED), /invalid IP address/);
  await assert.rejects(() => ipFor('loopback,nao-e-ip', '127.0.0.1', FORWARDED), /invalid IP address/);
  await assert.rejects(() => ipFor(['loopback', 'nope'], '127.0.0.1', FORWARDED), /invalid IP address/);
});

test('H1 tipos ja suportados continuam iguais', async () => {
  assert.equal(await ipFor(true, '8.8.8.8', FORWARDED), FORWARDED);
  assert.equal(await ipFor(false, '8.8.8.8', FORWARDED), '8.8.8.8');
  assert.equal(await ipFor(0, '127.0.0.1', FORWARDED), '127.0.0.1');
  assert.equal(await ipFor(1, '127.0.0.1', FORWARDED), FORWARDED);
  assert.equal(await ipFor((addr) => addr === '8.8.8.8', '8.8.8.8', FORWARDED), FORWARDED);
  assert.equal(await ipFor(['203.0.113.0/24'], '203.0.113.9', '198.18.0.1'), '198.18.0.1');
});
