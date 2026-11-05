import express from 'express';

const app = express();

export function listTrucks(req: any, res: any) {
  res.json([]);
}

app.get('/api/trucks', listTrucks);
